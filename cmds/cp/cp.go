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
	preserve    preserveSet
	force       bool
	noClobber   bool
	update      bool
	backup      bool
	suffix      string
	link        bool
	symlink     bool
	deref       bool
	derefArgs   bool
	attrsOnly   bool
	debug       bool
	oneFS       bool
	parents     bool
	removeDest  bool
	interactive bool
	verbose     bool
	failed      bool
	in          *bufio.Reader
	rootDev     devID
	haveRootDev bool
}

func run(rc *tool.RunContext, args []string) int {
	args = foldRShorthand(args)
	args = normalizeOptionalArgs(args)
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "r", false, "copy directories recursively (-R is identical to -r)")
	recursiveUpper := fs.BoolP("recursive-uppercase", "R", false, "copy directories recursively")
	archive := fs.BoolP("archive", "a", false, "same as -dR --preserve=all")
	preserveShort := fs.BoolP("preserve-short", "p", false, "preserve mode, ownership, timestamps")
	preserveList := fs.String("preserve", "", "preserve selected attributes: mode,ownership,timestamps,all")
	noPreserveList := fs.String("no-preserve", "", "do not preserve selected attributes")
	force := fs.BoolP("force", "f", false, "if an existing destination file cannot be opened, remove it and try again")
	noClobber := fs.BoolP("no-clobber", "n", false, "do not overwrite an existing file; silently skip it")
	interactive := fs.BoolP("interactive", "i", false, "prompt before overwrite")
	update := fs.BoolP("update", "u", false, "copy only when SOURCE is newer than the destination or destination is missing")
	targetDir := fs.StringP("target-directory", "t", "", "copy all SOURCE arguments into DIRECTORY")
	noTargetDir := fs.BoolP("no-target-directory", "T", false, "treat DEST as a normal file")
	backup := fs.StringP("backup", "b", "", "make a backup of each existing destination")
	fs.Lookup("backup").NoOptDefVal = "simple"
	suffix := fs.StringP("suffix", "S", "~", "override the usual backup suffix")
	link := fs.BoolP("link", "l", false, "hard link files instead of copying")
	symlink := fs.BoolP("symbolic-link", "s", false, "make symbolic links instead of copying")
	deref := fs.BoolP("dereference", "L", false, "always follow symbolic links in SOURCE")
	derefArgs := fs.BoolP("dereference-command-line", "H", false, "follow command-line symbolic links")
	noDeref := fs.BoolP("no-dereference", "P", false, "never follow symbolic links in SOURCE")
	fs.BoolP("no-dereference-preserve-links", "d", false, "same as --no-dereference --preserve=links")
	attrsOnly := fs.Bool("attributes-only", false, "copy only attributes, not file data")
	debug := fs.Bool("debug", false, "explain copy decisions on stderr")
	oneFS := fs.BoolP("one-file-system", "x", false, "stay on this file system during recursive copies")
	parents := fs.Bool("parents", false, "use full source file name under DIRECTORY")
	reflink := fs.String("reflink", "auto", "control clone/CoW copies: auto, always, never")
	removeDest := fs.Bool("remove-destination", false, "remove each existing destination before opening it")
	sparse := fs.String("sparse", "auto", "control sparse file creation: auto, always, never")
	fs.Bool("strip-trailing-slashes", false, "strip trailing slashes from operands")
	fs.Bool("copy-contents", false, "copy contents of special files when recursive (compatibility no-op)")
	fs.Bool("preserve-default-attributes", false, "preserve default attributes (compatibility no-op)")
	fs.BoolP("progress", "g", false, "accepted for compatibility; progress output is a no-op")
	fs.StringP("context", "Z", "", "accepted for compatibility; SELinux context is a no-op")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	operands = maybeStripTrailingSlashes(operands, fs.Changed("strip-trailing-slashes"))
	if err := validChoice("--reflink", *reflink, "auto", "always", "never"); err != "" {
		return tool.UsageError(rc, cmd, "%s", err)
	}
	if err := validChoice("--sparse", *sparse, "auto", "always", "never"); err != "" {
		return tool.UsageError(rc, cmd, "%s", err)
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

	preserve, preserveErr := parsePreserve(*preserveList)
	if preserveErr != "" {
		return tool.UsageError(rc, cmd, "%s", preserveErr)
	}
	if *preserveShort || *archive {
		preserve = allPreserve()
	}
	if noPreserve, preserveErr := parsePreserve(*noPreserveList); preserveErr != "" {
		return tool.UsageError(rc, cmd, "%s", preserveErr)
	} else {
		preserve.remove(noPreserve)
	}

	c := &copier{
		rc:          rc,
		recursive:   *recursive || *recursiveUpper || *archive,
		preserve:    preserve,
		force:       *force,
		noClobber:   *noClobber,
		update:      *update,
		backup:      *backup != "",
		suffix:      *suffix,
		link:        *link,
		symlink:     *symlink,
		deref:       *deref && !*archive,
		derefArgs:   *derefArgs,
		attrsOnly:   *attrsOnly,
		debug:       *debug,
		oneFS:       *oneFS,
		parents:     *parents,
		removeDest:  *removeDest,
		interactive: *interactive,
		verbose:     *verbose,
		in:          inputReader(rc.In),
	}
	if *noDeref || *archive || fs.Changed("no-dereference-preserve-links") {
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
			if c.parents {
				dst = filepath.Join(dest, parentPath(src))
			} else {
				dst = filepath.Join(dest, filepath.Base(src))
			}
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
	if c.recursive && !c.deref && !c.derefArgs {
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
		if c.oneFS {
			if dev, ok := fileDev(fi); ok {
				c.rootDev = dev
				c.haveRootDev = true
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
	if c.oneFS && c.haveRootDev {
		if dev, ok := fileDev(fi); ok && dev != c.rootDev {
			c.debugf("skipping '%s': different file system", src)
			return
		}
	}
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
	if c.preserve.any() {
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
		if c.removeDest {
			if err := os.Remove(dp); err != nil {
				c.errf("cannot remove '%s': %s", dst, reason(err))
				return
			}
		}
	} else if c.symlink {
		// Nothing to do before creating a new symbolic link.
	}
	if parent := filepath.Dir(dp); parent != "." && parent != dp {
		if err := os.MkdirAll(parent, 0o777); err != nil {
			c.errf("cannot create directory '%s': %s", filepath.Dir(dst), reason(err))
			return
		}
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
	flags := os.O_WRONLY | os.O_CREATE
	if !c.attrsOnly {
		flags |= os.O_TRUNC
	}
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
	if !c.attrsOnly {
		in, err := os.Open(sp)
		if err != nil {
			_ = out.Close()
			c.errf("cannot open '%s' for reading: %s", src, reason(err))
			return
		}
		_, werr := io.Copy(out, in)
		_ = in.Close()
		cerr := out.Close()
		if werr != nil {
			c.errf("error writing '%s': %s", dst, reason(werr))
			return
		}
		if cerr != nil {
			c.errf("error writing '%s': %s", dst, reason(cerr))
			return
		}
	} else if err := out.Close(); err != nil {
		c.errf("error writing '%s': %s", dst, reason(err))
		return
	}
	if c.preserve.any() {
		c.preserveAttrs(src, dst, fi)
	}
	c.debugf("copied '%s' -> '%s'", src, dst)
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
	c.debugf("copied symbolic link '%s' -> '%s'", src, dst)
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
	if c.preserve.mode {
		mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
		if err := os.Chmod(dp, mode); err != nil {
			c.errf("preserving permissions for '%s': %s", dst, reason(err))
		}
	}
	if c.preserve.ownership {
		preserveOwner(dp, fi)
	}
	if c.preserve.timestamps {
		if err := os.Chtimes(dp, atime(fi), fi.ModTime()); err != nil {
			c.errf("preserving times for '%s': %s", dst, reason(err))
		}
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

func (c *copier) debugf(format string, a ...any) {
	if c.debug {
		fmt.Fprintf(c.rc.Err, "cp: debug: "+format+"\n", a...)
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
		case a == "-Z" || a == "--context":
			out[i] = "--context="
		case a == "--backup":
			out[i] = "--backup=simple"
		case a == "--preserve":
			out[i] = "--preserve=mode,ownership,timestamps"
		case a == "--no-preserve":
			out[i] = "--no-preserve=all"
		case a == "--reflink":
			out[i] = "--reflink=always"
		case a == "--sparse":
			out[i] = "--sparse=auto"
		case a == "--interactive=always" || a == "--interactive=yes":
			out[i] = "--interactive"
		case a == "--interactive=never" || a == "--interactive=no" || a == "--interactive=none":
			out[i] = "--force"
		}
	}
	return out
}

func maybeStripTrailingSlashes(args []string, enabled bool) []string {
	if !enabled {
		return args
	}
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = strings.TrimRight(a, string(filepath.Separator)+"/")
		if out[i] == "" {
			out[i] = a
		}
	}
	return out
}

func parentPath(src string) string {
	clean := filepath.Clean(src)
	for strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		clean = strings.TrimPrefix(clean, ".."+string(filepath.Separator))
	}
	clean = strings.TrimPrefix(clean, string(filepath.Separator))
	if clean == "." || clean == "" {
		return filepath.Base(src)
	}
	return clean
}

type preserveSet struct {
	mode       bool
	ownership  bool
	timestamps bool
}

func allPreserve() preserveSet { return preserveSet{mode: true, ownership: true, timestamps: true} }

func (p preserveSet) any() bool { return p.mode || p.ownership || p.timestamps }

func (p *preserveSet) remove(other preserveSet) {
	if other.mode {
		p.mode = false
	}
	if other.ownership {
		p.ownership = false
	}
	if other.timestamps {
		p.timestamps = false
	}
}

func parsePreserve(s string) (preserveSet, string) {
	var p preserveSet
	if s == "" {
		return p, ""
	}
	for _, part := range strings.Split(s, ",") {
		switch strings.TrimSpace(part) {
		case "", "links", "context", "xattr":
		case "all":
			p = allPreserve()
		case "mode":
			p.mode = true
		case "ownership", "owner":
			p.ownership = true
		case "timestamps", "timestamp":
			p.timestamps = true
		default:
			return p, fmt.Sprintf("unsupported preserve attribute '%s'", part)
		}
	}
	return p, ""
}

func validChoice(flag, got string, allowed ...string) string {
	for _, a := range allowed {
		if got == a {
			return ""
		}
	}
	return fmt.Sprintf("invalid %s value '%s'", flag, got)
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
