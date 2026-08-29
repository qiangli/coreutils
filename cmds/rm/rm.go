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
	"github.com/qiangli/coreutils/pkg/locale"

	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode"

	"github.com/qiangli/coreutils/cmds/internal/pathops"
	"github.com/qiangli/coreutils/cmds/internal/rootguard"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

var isFilesystemRoot = rootguard.IsRoot

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
	isTerminal   bool
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
	operands, code := tool.ParseRequireOrder(rc, cmd, fs, args)
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
		isTerminal: isTerminal(rc.In),
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
	// POSIX and GNU require rejection based on the operand's final pathname
	// component, before any filesystem operation or prompt. filepath.Clean
	// erases a trailing "/." (and resolves "/.."), so inspecting its Base
	// could recurse into and remove a directory that the raw operand requires
	// us to reject without touching.
	base := finalOperandComponent(op)
	if base == "." || base == ".." {
		r.errf("refusing to remove '%s'", op)
		return
	}
	rp := r.rc.Path(op)
	fi, err := pathops.Lstat(rp)
	if err != nil {
		if r.force && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)) {
			return
		}
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	// A trailing separator requires the final component to resolve as a
	// directory. rc.Path normalizes that separator away, so retain the
	// operand-level signal for the preserve-root identity check.
	// POSIX Issue 7 (2016 edition) requires refusal of any operand that
	// resolves to the root directory before doing anything more with it;
	// -d reaches a removal attempt just as -r does, so the failsafe must
	// cover both (a bare `rm /` already stops at the Is-a-directory
	// diagnostic below without attempting removal).
	trailingSeparator := len(op) > 0 && os.IsPathSeparator(op[len(op)-1])
	if (r.recursive || r.dir) && r.preserveRoot && (fi.IsDir() || trailingSeparator) &&
		(isFilesystemRoot(rp, true) || (fi.IsDir() && rootguard.IsRootInfo(rp, fi))) {
		r.errf("it is dangerous to operate recursively on '%s'%s",
			op, rootguard.AliasSuffix(op, rp))
		return
	}
	if fi.IsDir() {
		if !r.recursive && !r.dir {
			r.errf("cannot remove '%s': Is a directory", op)
			return
		}
		if r.dir && !r.recursive {
			if r.shouldPrompt(rp, fi) && !r.confirm(op) {
				return
			}
			r.removeFile(op)
			return
		}
		// POSIX requires the write-protection prompt before descending into a
		// directory.  Waiting until after removeTree would let a negative reply
		// preserve the directory only after its children had already been
		// removed.
		if r.shouldPrompt(rp, fi) && !r.confirm(op) {
			return
		}
		r.removeTree(op)
		return
	}
	if r.shouldPrompt(rp, fi) && !r.confirm(op) {
		return
	}
	r.removeFile(op)
}

// finalOperandComponent returns the last non-empty pathname component without
// normalizing dot components. os.IsPathSeparator keeps this check correct for
// both accepted separator spellings on Windows and the POSIX separator.
func finalOperandComponent(op string) string {
	if volume := filepath.VolumeName(op); volume != "" {
		op = op[len(volume):]
	}
	end := len(op)
	for end > 0 && os.IsPathSeparator(op[end-1]) {
		end--
	}
	start := end
	for start > 0 && !os.IsPathSeparator(op[start-1]) {
		start--
	}
	return op[start:end]
}

func (r *remover) removeFile(op string) {
	if err := pathops.Remove(r.rc.Path(op)); err != nil {
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	r.verbosef("removed '%s'", op)
}

// removeTree removes a directory post-order, continuing past
// per-entry failures (the parent removal then reports its own
// error), matching GNU rm -r.
func (r *remover) removeTree(op string) {
	// The ordinary non-interactive lane needs no per-entry observation.
	// os.RemoveAll is implemented component-relatively on POSIX systems and
	// therefore avoids both PATH_MAX materialization and one descriptor per
	// depth. Keep the explicit walker below for prompts and verbose ordering.
	if !r.interactive && !r.verbose && (!r.isTerminal || r.force) {
		if err := pathops.RemoveAll(r.rc.Path(op)); err != nil {
			r.errf("cannot remove '%s': %s", op, reason(err))
		}
		return
	}

	root, err := pathops.OpenRoot(r.rc.Path(op))
	if err != nil {
		r.errf("cannot remove '%s': %s", op, reason(err))
		return
	}
	defer root.Close()

	type frame struct {
		rel, display string
		post         bool
		info         os.FileInfo
	}
	stack := []frame{{rel: ".", display: op}}
	for len(stack) > 0 {
		idx := len(stack) - 1
		current := stack[idx]
		stack = stack[:idx]

		if current.post {
			// The second directory prompt is specific to -i. The implicit
			// write-protection prompt was issued before descent.
			if r.interactive && !r.confirm(current.display) {
				continue
			}
			if current.rel == "." {
				// Root holds the directory open. Windows will not remove an open
				// directory, so close it before removing the operand itself.
				if err := root.Close(); err != nil {
					r.errf("cannot remove '%s': %s", current.display, reason(err))
					return
				}
				if err := pathops.Remove(r.rc.Path(op)); err != nil {
					r.errf("cannot remove '%s': %s", current.display, reason(err))
					return
				}
			} else if err := root.Remove(filepath.ToSlash(current.rel)); err != nil {
				r.errf("cannot remove '%s': %s", current.display, reason(err))
				continue
			}
			r.verbosef("removed directory '%s'", current.display)
			continue
		}

		if current.info != nil && !current.info.IsDir() {
			if r.shouldPrompt(r.rc.Path(current.display), current.info) && !r.confirm(current.display) {
				continue
			}
			if err := root.Remove(filepath.ToSlash(current.rel)); err != nil {
				r.errf("cannot remove '%s': %s", current.display, reason(err))
				continue
			}
			r.verbosef("removed '%s'", current.display)
			continue
		}
		if current.info != nil {
			resolved := r.rc.Path(current.display)
			if r.preserveRoot && (isFilesystemRoot(resolved, true) || rootguard.IsRootInfo(resolved, current.info)) {
				r.errf("it is dangerous to operate recursively on '%s'%s",
					current.display, rootguard.AliasSuffix(current.display, resolved))
				continue
			}
			if r.shouldPrompt(r.rc.Path(current.display), current.info) && !r.confirm(current.display) {
				continue
			}
		}

		dir, openErr := root.Open(filepath.ToSlash(current.rel))
		if openErr != nil {
			r.errf("cannot remove '%s': %s", current.display, reason(openErr))
			continue
		}
		entries, readErr := dir.ReadDir(-1)
		closeErr := dir.Close()
		if readErr != nil {
			r.errf("cannot remove '%s': %s", current.display, reason(readErr))
			continue
		}
		if closeErr != nil {
			r.errf("cannot remove '%s': %s", current.display, reason(closeErr))
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		stack = append(stack, frame{rel: current.rel, display: current.display, post: true})
		for i := len(entries) - 1; i >= 0; i-- {
			entry := entries[i]
			rel := filepath.Join(current.rel, entry.Name())
			display := filepath.Join(current.display, entry.Name())
			fi, statErr := root.Lstat(filepath.ToSlash(rel))
			if statErr != nil {
				r.errf("cannot remove '%s': %s", display, reason(statErr))
				continue
			}
			stack = append(stack, frame{rel: rel, display: display, info: fi})
		}
	}
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
	yes, matchErr := locale.MatchAffirmative(r.rc.Env, line)
	if matchErr != nil {
		r.errf("cannot interpret response: %s", matchErr)
		return false
	}
	return yes
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
		if optionRecognitionEnds(a) {
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
		if optionRecognitionEnds(a) {
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
		if optionRecognitionEnds(arg) {
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

// optionRecognitionEnds implements POSIX Utility Syntax Guideline 9 for rm:
// once the first operand is encountered, every following argument is an
// operand, even if it begins with '-' or is spelled "--".  Keep rm's option
// normalizers and the -f/-i precedence scan on the same boundary as
// ParseRequireOrder so they cannot reinterpret a pathname as an option.
func optionRecognitionEnds(arg string) bool {
	return arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-")
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

func (r *remover) shouldPrompt(rp string, fi os.FileInfo) bool {
	if r.force {
		return false
	}
	if r.interactive {
		return true
	}
	// The implicit write-protection prompt keys on the permissions of the
	// file being removed. A symbolic link is removed by unlink() on the
	// link itself, whose own permissions never deny writing; access(2)
	// would wrongly consult the referent (and fail for a dangling link),
	// so symlink operands never trigger the implicit prompt.
	if fi.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if r.isTerminal && !writableForPrompt(rp) {
		return true
	}
	return false
}

// Indirection keeps permission-sensitive prompt tests hermetic when the test
// process has privileges (notably root) that make access(2) succeed despite
// write bits being clear.
var writableForPrompt = isWritable

var isTerminal = func(r io.Reader) bool {
	if f, ok := r.(interface{ Fd() uintptr }); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	return false
}
