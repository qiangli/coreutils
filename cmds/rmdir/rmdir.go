// Package rmdircmd implements rmdir(1) per the GNU coreutils manual:
// remove the DIRECTORY(ies), if they are empty.
//
// Fresh implementation against the GNU manual
// (guonaihong/coreutils rmdir consulted as prior art; its -p removes
// recursively via os.RemoveAll, which does not match the documented
// "remove DIRECTORY and its ancestors" semantics).
package rmdircmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "rmdir",
	Synopsis: "Remove the DIRECTORY(ies), if they are empty.",
	Usage:    "rmdir [OPTION]... DIRECTORY...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type rm struct {
	rc             *tool.RunContext
	verbose        bool
	ignoreNonEmpty bool
	failed         bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	parents := fs.BoolP("parents", "p", false, "remove DIRECTORY and its ancestors; e.g., 'rmdir -p a/b' is similar to 'rmdir a/b a'")
	ignoreNonEmpty := fs.Bool("ignore-fail-on-non-empty", false, "ignore each failure that is solely because a directory is non-empty")
	verbose := fs.BoolP("verbose", "v", false, "output a diagnostic for every directory processed")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	r := &rm{rc: rc, verbose: *verbose, ignoreNonEmpty: *ignoreNonEmpty}
	for _, op := range operands {
		displayOp := op
		// Normalize slashes to the OS separator so the explicit
		// current-directory ("./") ancestor logic below is separator-
		// consistent on every platform. On Unix this is a no-op; on
		// Windows it rewrites an operand typed with "/" (e.g. "./a/b")
		// to native form so the -p walk still reaches ".". Keep the
		// original spelling for diagnostics: GNU reports the operand as
		// supplied, even when the host uses a different path separator.
		op = filepath.FromSlash(op)
		if !r.remove1(op, displayOp) {
			continue
		}
		if !*parents {
			continue
		}
		// -p: strip the operand one path component at a time and
		// remove each ancestor, stopping at the first failure. The
		// filesystem root itself is never attempted. Clean first so
		// a trailing separator does not yield the operand itself as
		// its own first "ancestor".
		cur := parentStart(op)
		for {
			parent := filepath.Dir(cur)
			if strings.HasPrefix(cur, "."+string(filepath.Separator)) && parent != "." && !strings.HasPrefix(parent, "."+string(filepath.Separator)) {
				parent = "." + string(filepath.Separator) + parent
			}
			if parent == cur || (parent == "." && !strings.HasPrefix(cur, "."+string(filepath.Separator))) || (parent != "." && filepath.Dir(parent) == parent) {
				break
			}
			cur = parent
			if !r.remove1(cur, cur) {
				break
			}
		}
	}
	if r.failed {
		return 1
	}
	return 0
}

// parentStart cleans trailing separators without discarding an explicit
// current-directory prefix. The prefix is significant to -p: for ./a/b,
// the current directory is an ancestor that rmdir must try after a and
// report if it cannot be removed.
func parentStart(op string) string {
	cur := filepath.Clean(op)
	if strings.HasPrefix(op, "."+string(filepath.Separator)) && cur != "." && !strings.HasPrefix(cur, "."+string(filepath.Separator)) {
		return "." + string(filepath.Separator) + cur
	}
	return cur
}

// remove1 removes one empty directory. op is the native filesystem path;
// displayOp preserves the user's spelling for diagnostics. The -v diagnostic
// is printed before the attempt, as GNU rmdir does.
func (r *rm) remove1(op, displayOp string) bool {
	if r.verbose {
		fmt.Fprintf(r.rc.Out, "rmdir: removing directory, '%s'\n", displayOp)
	}
	if op == "" {
		r.errf("failed to remove '': No such file or directory")
		return false
	}
	// POSIX rmdir must reject a path whose final component is "."
	// with EINVAL ("Invalid argument") on every platform — it is a portable
	// semantic guarantee, not an OS accident. This must happen BEFORE the
	// filesystem call: RunContext.Path normalizes a relative operand, so a
	// bare "." would otherwise resolve to the working directory itself and
	// (on some platforms, notably Windows, which silently strips a trailing
	// single dot during path canonicalization) let os.Remove succeed against
	// an otherwise-empty directory instead of failing as POSIX requires.
	//
	// A final component of ".." is deliberately NOT special-cased here.
	// POSIX's rmdir() only mandates EINVAL for dot; the errno for dot-dot is
	// unspecified, and real kernels (confirmed on Darwin, documented for
	// Linux's rmdir(2): "pathname has .. as its final component" under
	// ENOTEMPTY) simply let it fail naturally: the directory ".." resolves to
	// always still contains the child entry the operand traversed through, so
	// it can never be empty. Hardcoding EINVAL here previously produced the
	// wrong diagnostic and, worse, bypassed --ignore-fail-on-non-empty, which
	// must suppress this exactly like any other non-empty-directory failure.
	//
	// The base is taken from the uncleaned native path, NOT from
	// filepath.Clean(op): Clean collapses "a/." to "a", silently swallowing
	// the trailing dot that POSIX mandates the kernel reject. Separator
	// normalization preserves path components, so "a/." and "a/./" are both
	// caught here.
	if base := filepath.Base(op); base == "." {
		r.errf("failed to remove '%s': Invalid argument", displayOp)
		return false
	}
	rp := r.rc.Path(op)
	fi, err := os.Lstat(rp)
	if err != nil {
		r.errf("failed to remove '%s': %s", displayOp, reason(err))
		return false
	}
	if !fi.IsDir() {
		r.errf("failed to remove '%s': Not a directory", displayOp)
		return false
	}
	if err := os.Remove(rp); err != nil {
		if r.ignoreNonEmpty && isNonEmpty(err) {
			return false
		}
		r.errf("failed to remove '%s': %s", displayOp, reason(err))
		return false
	}
	return true
}

func isNonEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) || isNonEmptySys(err)
}

func (r *rm) errf(format string, a ...any) {
	fmt.Fprintf(r.rc.Err, "rmdir: "+format+"\n", a...)
	r.failed = true
}

// reason unwraps err to its root cause and capitalizes the first
// letter, matching the strerror() shape GNU diagnostics use
// ("Directory not empty").
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
	rs := []rune(s)
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}
