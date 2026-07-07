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
	"bufio"
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
	rc          *tool.RunContext
	recursive   bool
	preserve    bool
	force       bool
	noClobber   bool
	update      bool
	backup      bool
	suffix      string
	link        bool
	symlink     bool
	deref       bool
	interactive bool
	verbose     bool
	failed      bool
	in          *bufio.Reader
}

func run(rc *tool.RunContext, args []string) int {
	args = foldRShorthand(args)
	args = normalizeOptionalArgs(args)
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "r", false, "copy directories recursively (-R is identical to -r)")
	preserve := fs.BoolP("preserve", "p", false, "preserve mode, ownership, timestamps")
	force := fs.BoolP("force", "f", false, "if an existing destination file cannot be opened, remove it and try again")
	noClobber := fs.BoolP("no-clobber", "n", false, "do not overwrite an existing file; silently skip it")
	interactive := fs.BoolP("interactive", "i", false, "prompt before overwrite")
	update := fs.BoolP("update", "u", false, "copy only when SOURCE is newer than the destination or destination is missing")
	targetDir := fs.StringP("target-directory", "t", "", "copy all SOURCE arguments into DIRECTORY")
	noTargetDir := fs.BoolP("no-target-directory", "T", false, "treat DEST as a normal file")
	backup := fs.String("backup", "", "make a backup of each existing destination")
	suffix := fs.StringP("suffix", "S", "~", "override the usual backup suffix")
	link := fs.BoolP("link", "l", false, "hard link files instead of copying")
	symlink := fs.BoolP("symbolic-link", "s", false, "make symbolic links instead of copying")
	deref := fs.BoolP("dereference", "L", false, "always follow symbolic links in SOURCE")
	noDeref := fs.BoolP("no-dereference", "P", false, "never follow symbolic links in SOURCE")
	fs.Bool("progress", false, "accepted for compatibility; progress output is a no-op")
	fs.String("context", "", "accepted for compatibility; SELinux context is a no-op")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *targetDir != "" && *noTargetDir {
		return tool.UsageError(rc, cmd, "cannot combine --target-directory and --no-target-directory")
	}
	if *link && *symlink {
		return tool.UsageError(rc, cmd, "cannot make both hard and symbolic links")
	}
	switch len(operands) {
	case 0:
		return tool.UsageError(rc, cmd, "missing file operand")
	case 1:
		if *targetDir == "" {
			return tool.UsageError(rc, cmd, "missing destination file operand after '%s'", operands[0])
		}
	}

	c := &copier{
		rc:          rc,
		recursive:   *recursive,
		preserve:    *preserve,
		force:       *force,
		noClobber:   *noClobber,
		update:      *update,
		backup:      *backup != "",
		suffix:      *suffix,
		link:        *link,
		symlink:     *symlink,
		deref:       *deref,
		interactive: *interactive,
		verbose:     *verbose,
		in:          inputReader(rc.In),
	}
	if *noDeref {
		c.deref = false
	}
	// GNU rule: of -f and -n, the one given last takes effect.
	switch lastOverride(args) {
	case 'f':
		c.noClobber = false
		c.interactive = false
	case 'n':
		c.force = false
		c.interactive = false
	case 'i':
		c.force = false
		c.noClobber = false
	}

	dest := ""
	srcs := operands
	if *targetDir != "" {
		dest = *targetDir
	} else {
		dest = operands[len(operands)-1]
		srcs = operands[:len(operands)-1]
	}
	di, err := os.Stat(rc.Path(dest))
	todir := !*noTargetDir && err == nil && di.IsDir()
	if *targetDir != "" && !todir {
		fmt.Fprintf(rc.Err, "cp: target directory '%s' is not a directory\n", dest)
		return 1
	}
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
	if c.recursive && !c.deref {
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
		if c.update && !sourceNewer(sp, dp) {
			return
		}
		if c.interactive && !c.confirm(dst) {
			return
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
		if c.backup && !c.backupDest(dst) {
			return
		}
	} else if c.symlink {
		// Nothing to do before creating a new symbolic link.
	}
	if c.link {
		if err := os.Link(sp, dp); err != nil {
			c.errf("cannot create hard link '%s' to '%s': %s", dst, src, reason(err))
			return
		}
		c.verbosef("'%s' -> '%s'", src, dst)
		return
	}
	if c.symlink {
		if err := os.Symlink(src, dp); err != nil {
			c.errf("cannot create symbolic link '%s' to '%s': %s", dst, src, reason(err))
			return
		}
		c.verbosef("'%s' -> '%s'", src, dst)
		return
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
		if c.update && !sourceNewer(sp, dp) {
			return
		}
		if c.interactive && !c.confirm(dst) {
			return
		}
		if c.backup && !c.backupDest(dst) {
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

func (c *copier) confirm(dst string) bool {
	fmt.Fprintf(c.rc.Err, "cp: overwrite '%s'? ", dst)
	line, err := c.in.ReadString('\n')
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

func (c *copier) backupDest(dst string) bool {
	dp := c.rc.Path(dst)
	bp := dp + c.suffix
	_ = os.Remove(bp)
	if err := os.Rename(dp, bp); err != nil {
		c.errf("cannot backup '%s': %s", dst, reason(err))
		return false
	}
	return true
}

func sourceNewer(src, dst string) bool {
	si, serr := os.Stat(src)
	di, derr := os.Stat(dst)
	if serr != nil || derr != nil {
		return true
	}
	return si.ModTime().After(di.ModTime())
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

func normalizeOptionalArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i, a := range out {
		if a == "--" {
			break
		}
		switch {
		case a == "--backup":
			out[i] = "--backup=simple"
		case a == "--interactive=always" || a == "--interactive=yes":
			out[i] = "--interactive"
		case a == "--interactive=never" || a == "--interactive=no" || a == "--interactive=none":
			out[i] = "--force"
		}
	}
	return out
}

// lastOverride reports which of -f / -n appeared last on the command
// line (GNU: "If you specify more than one of -i, -f, -n, only the
// final one takes effect"). Returns 'f', 'n', 'i', or 0.
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
		case a == "--interactive" || strings.HasPrefix(a, "--interactive="):
			last = 'i'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, ch := range a[1:] {
				if ch == 'f' {
					last = 'f'
				}
				if ch == 'n' {
					last = 'n'
				}
				if ch == 'i' {
					last = 'i'
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
