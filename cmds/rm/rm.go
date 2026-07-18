// Package rmcmd implements rm(1) per the GNU coreutils manual: remove
// files or directories.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/rm
// (BSD-3-Clause).
// Changes: rewired to the tool framework; manual post-order tree
// removal for GNU -v output and per-file error continuation; GNU
// root-protection failsafe (--preserve-root default).
package rmcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "rm",
	Synopsis: "Remove (unlink) the FILE(s).",
	Usage:    "rm [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type remover struct {
	rc           *tool.RunContext
	recursive    bool
	force        bool
	dir          bool
	interactive  bool
	preserveRoot bool
	verbose      bool
	failed       bool
	in           *bufio.Reader
}

func run(rc *tool.RunContext, args []string) int {
	args = foldRShorthand(args)
	args = normalizeOptionalArgs(args)
	lastPromptOption := lastPromptOption(args)

	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "r", false, "remove directories and their contents recursively (-R is identical to -r)")
	dir := fs.BoolP("dir", "d", false, "remove empty directories")
	force := fs.BoolP("force", "f", false, "ignore nonexistent files and arguments, never prompt")
	interactive := fs.BoolP("interactive", "i", false, "prompt before every removal")
	interactiveOnce := fs.BoolP("interactive-once", "I", false, "prompt once before removing recursively or more than three files")
	preserveRoot := fs.Bool("preserve-root", true, "do not remove '/'")
	noPreserveRoot := fs.Bool("no-preserve-root", false, "do not treat '/' specially")
	fs.BoolP("one-file-system", "o", false, "accepted for compatibility; filesystem boundary pruning is a no-op")
	fs.BoolP("progress", "g", false, "accepted for compatibility; progress output is a no-op")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	forceMode, interactiveMode := *force, *interactive
	switch lastPromptOption {
	case promptForce:
		interactiveMode = false
	case promptInteractive:
		forceMode = false
	}
	if len(operands) == 0 {
		if forceMode {
			return 0
		}
		return tool.UsageError(rc, cmd, "missing operand")
	}

	ask := (interactiveMode || *interactiveOnce) && !forceMode
	if *interactiveOnce && !*interactive && len(operands) <= 3 && !*recursive {
		ask = false
	}
	r := &remover{
		rc: rc, recursive: *recursive, force: forceMode, dir: *dir,
		interactive: ask, preserveRoot: *preserveRoot && !*noPreserveRoot,
		verbose: *verbose, in: inputReader(rc.In),
	}
	for _, op := range operands {
		r.remove(op)
	}
	if r.failed {
		return 1
	}
	return 0
}

func (r *remover) remove(op string) {
	if op == "" {
		if r.force {
			return
		}
		r.errf("cannot remove '': No such file or directory")
		return
	}
	rp := r.rc.Path(op)
	base := filepath.Base(filepath.Clean(op))
	if base == "." || base == ".." {
		r.errf("refusing to remove '%s'", op)
		return
	}
	fi, err := os.Lstat(rp)
	if err != nil {
		if r.force && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)) {
			return
		}
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	if fi.IsDir() {
		if !r.recursive && !r.dir {
			r.errf("cannot remove '%s': Is a directory", op)
			return
		}
		// GNU --preserve-root default: refuse to operate on the
		// filesystem root unless --no-preserve-root was explicit.
		if r.preserveRoot && filepath.Dir(filepath.Clean(rp)) == filepath.Clean(rp) {
			r.errf("it is dangerous to operate recursively on '%s'", op)
			return
		}
		if r.interactive && !r.confirm(op) {
			return
		}
		if r.dir && !r.recursive {
			r.removeFile(op)
			return
		}
		r.removeTree(op)
		return
	}
	if r.interactive && !r.confirm(op) {
		return
	}
	r.removeFile(op)
}

func (r *remover) removeFile(op string) {
	if err := os.Remove(r.rc.Path(op)); err != nil {
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	r.verbosef("removed '%s'", op)
}

// removeTree removes a directory post-order, continuing past
// per-entry failures (the parent removal then reports its own
// error), matching GNU rm -r.
func (r *remover) removeTree(op string) {
	entries, err := os.ReadDir(r.rc.Path(op))
	if err != nil {
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	for _, e := range entries {
		child := filepath.Join(op, e.Name())
		r.remove(child)
	}

	if r.interactive && !r.confirm(op) {
		return
	}

	if err := os.Remove(r.rc.Path(op)); err != nil {
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	r.verbosef("removed directory '%s'", op)
}

func (r *remover) errf(format string, a ...any) {
	fmt.Fprintf(r.rc.Err, "rm: "+format+"\n", a...)
	r.failed = true
}

func (r *remover) verbosef(format string, a ...any) {
	if r.verbose {
		fmt.Fprintf(r.rc.Out, format+"\n", a...)
	}
}

func (r *remover) confirm(op string) bool {
	fmt.Fprintf(r.rc.Err, "rm: remove '%s'? ", op)
	line, err := r.in.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	line = strings.TrimSpace(line)
	return line == "y" || line == "Y" || strings.EqualFold(line, "yes")
}

func inputReader(r io.Reader) *bufio.Reader {
	if r == nil {
		r = strings.NewReader("")
	}
	return bufio.NewReader(r)
}

// foldRShorthand rewrites -R into -r inside short-option clusters
// (before any "--" terminator). GNU rm treats -R and -r identically;
// pflag cannot attach two shorthands to one flag and inventing a
// long name for -R is forbidden, so the alias is folded before Parse.
// Safe because every rm short flag is a boolean.
func foldRShorthand(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if a == "--" {
			break
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' {
			out[i] = strings.ReplaceAll(a, "R", "r")
		}
	}
	return out
}

func normalizeOptionalArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if a == "--" {
			break
		}
		switch {
		case a == "--interactive=always" || a == "--interactive=yes":
			out[i] = "--interactive"
		case a == "--interactive=once":
			out[i] = "--interactive-once"
		case a == "--interactive=never" || a == "--interactive=no" || a == "--interactive=none":
			out[i] = "--force"
		case a == "--preserve-root=all":
			out[i] = "--preserve-root"
		}
	}
	return out
}

type promptOption int

const (
	promptNone promptOption = iota
	promptForce
	promptInteractive
)

// lastPromptOption implements the POSIX rule that -f and -i override each
// other according to their last occurrence, including within short clusters.
func lastPromptOption(args []string) promptOption {
	last := promptNone
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "--force":
			last = promptForce
		case "--interactive":
			last = promptInteractive
		default:
			if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
				for _, flag := range arg[1:] {
					switch flag {
					case 'f':
						last = promptForce
					case 'i':
						last = promptInteractive
					}
				}
			}
		}
	}
	return last
}

// reason unwraps err to its root cause and capitalizes the first
// letter, matching the strerror() shape GNU diagnostics use.
func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		err = se.Err
	}
	s := err.Error()
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
