// Package rmcmd implements rm(1) per the GNU coreutils manual: remove
// files or directories.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/rm
// (BSD-3-Clause).
// Changes: rewired to the tool framework; manual post-order tree
// removal for GNU -v output and per-file error continuation; GNU
// root-protection failsafe (--preserve-root default); the -i/-I/
// --interactive family is refused (interactive prompting is out of
// scope for an agent userland).
package rmcmd

import (
	"errors"
	"fmt"
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
	rc        *tool.RunContext
	recursive bool
	force     bool
	verbose   bool
	failed    bool
}

func run(rc *tool.RunContext, args []string) int {
	// The interactive family is documented GNU behavior we
	// deliberately do not cover: an agent userland never prompts.
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--interactive" || strings.HasPrefix(a, "--interactive=") {
			return tool.NotSupported(rc, cmd, "--interactive (prompting requires an interactive user; this agent userland never prompts)")
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(a[1:], "iI") {
			return tool.NotSupported(rc, cmd, "-i/-I (interactive prompting; this agent userland never prompts)")
		}
	}
	args = foldRShorthand(args)

	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "r", false, "remove directories and their contents recursively (-R is identical to -r)")
	force := fs.BoolP("force", "f", false, "ignore nonexistent files and arguments, never prompt")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		if *force {
			return 0 // GNU: rm -f with no operands succeeds silently
		}
		return tool.UsageError(rc, cmd, "missing operand")
	}

	r := &remover{rc: rc, recursive: *recursive, force: *force, verbose: *verbose}
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
	fi, err := os.Lstat(rp)
	if err != nil {
		if r.force && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)) {
			return
		}
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	if fi.IsDir() {
		if !r.recursive {
			r.errf("cannot remove '%s': Is a directory", op)
			return
		}
		// GNU --preserve-root default: refuse to operate recursively
		// on the filesystem root. --no-preserve-root is deliberately
		// not implemented.
		if cleaned := filepath.Clean(rp); filepath.Dir(cleaned) == cleaned {
			r.errf("it is dangerous to operate recursively on '%s'", op)
			fmt.Fprintf(r.rc.Err, "rm: --no-preserve-root is not supported by pure-Go coreutils\n")
			return
		}
		r.removeTree(op)
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
		if e.IsDir() {
			r.removeTree(child)
		} else {
			r.removeFile(child)
		}
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
