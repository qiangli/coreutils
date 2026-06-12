// Package cpcmd implements cp(1) per the GNU coreutils manual: copy
// SOURCE to DEST, or multiple SOURCE(s) to DIRECTORY.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/cp and pkg/cp (BSD-3-Clause).
// Changes: rewired to the tool framework; added GNU -p preservation,
// -f/-n semantics, -v output shape, dir-into-itself detection, and
// per-file error continuation.
package cpcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "cp",
	Synopsis: "Copy SOURCE to DEST, or multiple SOURCE(s) to DIRECTORY.",
	Usage:    "cp [OPTION]... SOURCE DEST\n   or: cp [OPTION]... SOURCE... DIRECTORY",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type copier struct {
	rc        *tool.RunContext
	recursive bool
	preserve  bool
	force     bool
	noClobber bool
	verbose   bool
	failed    bool
}

func run(rc *tool.RunContext, args []string) int {
	args = foldRShorthand(args)
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "r", false, "copy directories recursively (-R is identical to -r)")
	preserve := fs.BoolP("preserve", "p", false, "preserve mode, ownership, timestamps")
	force := fs.BoolP("force", "f", false, "if an existing destination file cannot be opened, remove it and try again")
	noClobber := fs.BoolP("no-clobber", "n", false, "do not overwrite an existing file; silently skip it")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	switch len(operands) {
	case 0:
		return tool.UsageError(rc, cmd, "missing file operand")
	case 1:
		return tool.UsageError(rc, cmd, "missing destination file operand after '%s'", operands[0])
	}

	c := &copier{
		rc:        rc,
		recursive: *recursive,
		preserve:  *preserve,
		force:     *force,
		noClobber: *noClobber,
		verbose:   *verbose,
	}
	// GNU rule: of -f and -n, the one given last takes effect.
	switch lastOverride(args) {
	case 'f':
		c.noClobber = false
	case 'n':
		c.force = false
	}

	dest := operands[len(operands)-1]
	srcs := operands[:len(operands)-1]
	di, err := os.Stat(rc.Path(dest))
	todir := err == nil && di.IsDir()
	if len(srcs) > 1 && !todir {
		fmt.Fprintf(rc.Err, "cp: target '%s' is not a directory\n", dest)
		return 1
	}
	for _, src := range srcs {
		dst := dest
		if todir {
			dst = filepath.Join(dest, filepath.Base(src))
		}
		c.copyEntry(src, dst)
	}
	if c.failed {
		return 1
	}
	return 0
}

// copyEntry dispatches one SOURCE operand. Without -r symlinks are
// followed (os.Stat); with -r they are copied as symlinks, per the
// GNU manual's -R default.
func (c *copier) copyEntry(src, dst string) {
	if src == "" {
		c.errf("cannot stat '': No such file or directory")
		return
	}
	stat := os.Stat
	if c.recursive {
		stat = os.Lstat
	}
	fi, err := stat(c.rc.Path(src))
	if err != nil {
		c.errf("cannot stat '%s': %s", src, reason(err))
		return
	}
	switch {
	case fi.IsDir():
		if !c.recursive {
			c.errf("-r not specified; omitting directory '%s'", src)
			return
		}
		absSrc, e1 := filepath.Abs(c.rc.Path(src))
		absDst, e2 := filepath.Abs(c.rc.Path(dst))
		if e1 == nil && e2 == nil {
			if absDst == absSrc {
				c.errf("'%s' and '%s' are the same file", src, dst)
				return
			}
			if strings.HasPrefix(absDst, absSrc+string(filepath.Separator)) {
				c.errf("cannot copy a directory, '%s', into itself, '%s'", src, dst)
				return
			}
		}
		c.copyDir(src, dst, fi)
	case fi.Mode()&os.ModeSymlink != 0:
		c.copySymlink(src, dst)
	default:
		c.copyFile(src, dst, fi)
	}
}

func (c *copier) copyDir(src, dst string, fi os.FileInfo) {
	created := false
	if di, err := os.Lstat(c.rc.Path(dst)); err == nil {
		if !di.IsDir() {
			c.errf("cannot overwrite non-directory '%s' with directory '%s'", dst, src)
			return
		}
	} else {
		// Created writable regardless of the source mode so children
		// can land; the final mode is applied after the tree is
		// populated (the GNU manual's read-only-source-dir behavior).
		if err := os.Mkdir(c.rc.Path(dst), fi.Mode().Perm()|0o700); err != nil {
			c.errf("cannot create directory '%s': %s", dst, reason(err))
			return
		}
		created = true
	}
	c.verbosef("'%s' -> '%s'", src, dst)
	entries, err := os.ReadDir(c.rc.Path(src))
	if err != nil {
		c.errf("cannot access '%s': %s", src, reason(err))
	} else {
		for _, e := range entries {
			csrc := filepath.Join(src, e.Name())
			cdst := filepath.Join(dst, e.Name())
			ci, err := os.Lstat(c.rc.Path(csrc))
			if err != nil {
				c.errf("cannot stat '%s': %s", csrc, reason(err))
				continue
			}
			switch {
			case ci.IsDir():
				c.copyDir(csrc, cdst, ci)
			case ci.Mode()&os.ModeSymlink != 0:
				c.copySymlink(csrc, cdst)
			default:
				c.copyFile(csrc, cdst, ci)
			}
		}
	}
	if c.preserve {
		c.preserveAttrs(src, dst, fi)
	} else if created {
		if err := os.Chmod(c.rc.Path(dst), fi.Mode().Perm()); err != nil {
			c.errf("setting permissions for '%s': %s", dst, reason(err))
		}
	}
}

func (c *copier) copyFile(src, dst string, fi os.FileInfo) {
	sp, dp := c.rc.Path(src), c.rc.Path(dst)
	if _, err := os.Lstat(dp); err == nil {
		if c.noClobber {
			return // -n: silently skip; exit status unaffected
		}
		if ds, err := os.Stat(dp); err == nil {
			if ss, err := os.Stat(sp); err == nil && os.SameFile(ss, ds) {
				c.errf("'%s' and '%s' are the same file", src, dst)
				return
			}
			if ds.IsDir() {
				c.errf("cannot overwrite directory '%s' with non-directory", dst)
				return
			}
		}
	}
	in, err := os.Open(sp)
	if err != nil {
		c.errf("cannot open '%s' for reading: %s", src, reason(err))
		return
	}
	defer in.Close()
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	out, err := os.OpenFile(dp, flags, fi.Mode().Perm())
	if err != nil && c.force {
		// -f: if an existing destination file cannot be opened,
		// remove it and try again.
		if os.Remove(dp) == nil {
			out, err = os.OpenFile(dp, flags, fi.Mode().Perm())
		}
	}
	if err != nil {
		c.errf("cannot create regular file '%s': %s", dst, reason(err))
		return
	}
	_, werr := io.Copy(out, in)
	cerr := out.Close()
	if werr != nil {
		c.errf("error writing '%s': %s", dst, reason(werr))
		return
	}
	if cerr != nil {
		c.errf("error writing '%s': %s", dst, reason(cerr))
		return
	}
	if c.preserve {
		c.preserveAttrs(src, dst, fi)
	}
	c.verbosef("'%s' -> '%s'", src, dst)
}

func (c *copier) copySymlink(src, dst string) {
	sp, dp := c.rc.Path(src), c.rc.Path(dst)
	target, err := os.Readlink(sp)
	if err != nil {
		c.errf("cannot read symbolic link '%s': %s", src, reason(err))
		return
	}
	if _, err := os.Lstat(dp); err == nil {
		if c.noClobber {
			return
		}
		if err := os.Remove(dp); err != nil {
			c.errf("cannot remove '%s': %s", dst, reason(err))
			return
		}
	}
	if err := os.Symlink(target, dp); err != nil {
		c.errf("cannot create symbolic link '%s': %s", dst, reason(err))
		return
	}
	c.verbosef("'%s' -> '%s'", src, dst)
}

// preserveAttrs implements -p: mode, ownership, timestamps. Failing
// to preserve ownership without the needed privilege is not an error
// (GNU -p rule); mode/time failures are diagnosed.
func (c *copier) preserveAttrs(src, dst string, fi os.FileInfo) {
	dp := c.rc.Path(dst)
	mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err := os.Chmod(dp, mode); err != nil {
		c.errf("preserving permissions for '%s': %s", dst, reason(err))
	}
	preserveOwner(dp, fi)
	if err := os.Chtimes(dp, atime(fi), fi.ModTime()); err != nil {
		c.errf("preserving times for '%s': %s", dst, reason(err))
	}
	_ = src
}

func (c *copier) errf(format string, a ...any) {
	fmt.Fprintf(c.rc.Err, "cp: "+format+"\n", a...)
	c.failed = true
}

func (c *copier) verbosef(format string, a ...any) {
	if c.verbose {
		fmt.Fprintf(c.rc.Out, format+"\n", a...)
	}
}

// foldRShorthand rewrites -R into -r inside short-option clusters
// (before any "--" terminator). GNU cp treats -R and -r identically;
// pflag cannot attach two shorthands to one flag and inventing a
// long name for -R is forbidden, so the alias is folded before Parse.
// Safe because every cp short flag is a boolean (no cluster carries a
// value that could contain an R).
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

// lastOverride reports which of -f / -n appeared last on the command
// line (GNU: "If you specify more than one of -i, -f, -n, only the
// final one takes effect"). Returns 'f', 'n', or 0.
func lastOverride(args []string) byte {
	var last byte
	for _, a := range args {
		if a == "--" {
			break
		}
		switch {
		case a == "--force":
			last = 'f'
		case a == "--no-clobber":
			last = 'n'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, ch := range a[1:] {
				if ch == 'f' {
					last = 'f'
				}
				if ch == 'n' {
					last = 'n'
				}
			}
		}
	}
	return last
}

// reason unwraps err to its root cause and capitalizes the first
// letter, matching the strerror() shape GNU diagnostics use
// ("No such file or directory").
func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
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
