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
	"path/filepath"
	"strings"
	"syscall"

	"github.com/qiangli/coreutils/cmds/internal/pathops"
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
	operands, code := tool.ParseRequireOrder(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	r := &rm{rc: rc, verbose: *verbose, ignoreNonEmpty: *ignoreNonEmpty}
	for _, op := range operands {
		if !r.remove1(op) {
			continue
		}
		if !*parents {
			continue
		}
		// -p: strip the operand one path component at a time and
		// remove each ancestor, stopping at the first failure. The
		// filesystem root itself is never attempted. Clean first so
		// a trailing separator does not yield the operand itself as
		// its own first "ancestor". Every step stays in the operand's
		// OWN spelling (tool.OperandDir/OperandClean, and the separator
		// the caller typed): rewriting /tmp/a/b into the host's native
		// \tmp\a\b would take the ancestors out of the Windows mount
		// table, and the diagnostic must name the operand as supplied.
		sep := tool.OperandSeparator(op)
		cur := parentStart(op, sep)
		for {
			parent := tool.OperandDir(cur)
			if strings.HasPrefix(cur, "."+sep) && parent != "." && !strings.HasPrefix(parent, "."+sep) {
				parent = "." + sep + parent
			}
			if parent == cur || (parent == "." && !strings.HasPrefix(cur, "."+sep)) || (parent != "." && tool.OperandDir(parent) == parent) {
				break
			}
			cur = parent
			if !r.remove1(cur) {
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
func parentStart(op, sep string) string {
	cur := tool.OperandClean(op)
	if strings.HasPrefix(op, "."+sep) && cur != "." && !strings.HasPrefix(cur, "."+sep) {
		return "." + sep + cur
	}
	return cur
}

// remove1 removes one empty directory. op is the operand in the caller's own
// spelling — it is both what the diagnostics print and what rawOperandPath
// converts, so a Windows operand still resolves through the mount table. The
// -v diagnostic is printed before the attempt, as GNU rmdir does.
func (r *rm) remove1(op string) bool {
	if r.verbose {
		fmt.Fprintf(r.rc.Out, "rmdir: removing directory, '%s'\n", op)
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
	// POSIX requires the removal to fail but does not prescribe its errno.
	// Preserve the component for the host's pathname walk so an invalid prefix
	// still fails with its native error (for example ENOENT for missing/.. or
	// ENOTDIR for file/..) and a valid child/.. gets the host's native result.
	//
	// The base is taken from the uncleaned native path, NOT from
	// filepath.Clean(op): Clean collapses "a/." to "a", silently swallowing
	// the trailing dot that POSIX mandates the kernel reject. Separator
	// normalization preserves path components, so "a/." and "a/./" are both
	// caught here.
	if base := filepath.Base(op); base == "." {
		r.errf("failed to remove '%s': Invalid argument", op)
		return false
	}
	rp := rawOperandPath(r.rc, op)
	fi, err := pathops.Lstat(rp)
	if err != nil {
		r.errf("failed to remove '%s': %s", op, reason(err))
		return false
	}
	if !fi.IsDir() {
		r.errf("failed to remove '%s': Not a directory", op)
		return false
	}
	if err := pathops.Remove(rp); err != nil {
		if r.ignoreNonEmpty && isNonEmpty(err) {
			return false
		}
		r.errf("failed to remove '%s': %s", op, reason(err))
		return false
	}
	return true
}

// rawOperandPath resolves a relative operand under the invocation directory
// without filepath.Join's lexical Clean. The kernel must see every pathname
// component: cleaning f/.. or missing/.. would incorrectly turn either into
// the working directory and could remove that directory instead of reporting
// the invalid prefix. Absolute operands already carry their own base.
func rawOperandPath(rc *tool.RunContext, operand string) string {
	// An absolute operand in the shell's spelling (/tmp/x, or on Windows the
	// MSYS drive form bashy hands out, /c/Users/…) resolves to its native
	// form; only a RELATIVE operand is kept raw for the "." semantics above.
	// RunContext.RawPath is exactly that rule.
	return rc.RawPath(operand)
}

func isNonEmpty(err error) bool {
	return errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) || isNonEmptySys(err)
}

func (r *rm) errf(format string, a ...any) {
	fmt.Fprintf(r.rc.Err, "rmdir: "+format+"\n", a...)
	r.failed = true
}

// reason renders the filesystem cause of err the way GNU does: the errno
// text with its first letter capitalized, with the os wrappers unwrapped so
// the caller's own "<tool>: <name>: " prefix is not doubled. tool.SysErrString
// is the one implementation; on Windows it also maps the OS's own sentence
// ("The system cannot find the file specified.") onto the POSIX strerror
// wording every GNU diagnostic — and bash's fixtures — expect.
func reason(err error) string {
	return tool.SysErrString(err)
}
