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

	"github.com/qiangli/coreutils/pkg/locale"
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

var fileAtime = atime

type copier struct {
	rc           *tool.RunContext
	paths        *pathResolver
	recursive    bool
	preserve     preserveSet
	force        bool
	noClobber    bool
	update       bool
	backup       bool
	suffix       string
	link         bool
	symlink      bool
	derefMode    dereferenceMode
	attrsOnly    bool
	copyContents bool
	debug        bool
	oneFS        bool
	parents      bool
	removeDest   bool
	interactive  bool
	verbose      bool
	failed       bool
	in           *bufio.Reader
	rootDev      devID
	haveRootDev  bool
	dirStack     []os.FileInfo
	umask        os.FileMode
}

func run(rc *tool.RunContext, args []string) int {
	args = foldRShorthand(args)
	args = normalizeOptionalArgs(args)
	fs := tool.NewFlags(cmd.Name)
	if envPresent(rc.Env, "POSIXLY_CORRECT") {
		// POSIX Utility Syntax Guideline 9: once the operands begin, a later
		// "-"-looking argument is itself an operand (e.g. a destination
		// directory literally named "-p"), not a new option. GNU getopt
		// applies this REQUIRE_ORDER rule
		// exactly when POSIXLY_CORRECT is set; without it, GNU permits
		// options anywhere (the tool.NewFlags default), so only disable
		// permutation in POSIX mode.
		fs.SetInterspersed(false)
	}
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
	fs.BoolP("dereference", "L", false, "always follow symbolic links in SOURCE")
	fs.BoolP("dereference-command-line", "H", false, "follow command-line symbolic links")
	fs.BoolP("no-dereference", "P", false, "never follow symbolic links in SOURCE")
	fs.BoolP("no-dereference-preserve-links", "d", false, "same as --no-dereference --preserve=links")
	attrsOnly := fs.Bool("attributes-only", false, "copy only attributes, not file data")
	debug := fs.Bool("debug", false, "explain copy decisions on stderr")
	oneFS := fs.BoolP("one-file-system", "x", false, "stay on this file system during recursive copies")
	parents := fs.Bool("parents", false, "use full source file name under DIRECTORY")
	reflink := fs.String("reflink", "auto", "control clone/CoW copies: auto, always, never")
	removeDest := fs.Bool("remove-destination", false, "remove each existing destination before opening it")
	sparse := fs.String("sparse", "auto", "control sparse file creation: auto, always, never")
	fs.Bool("strip-trailing-slashes", false, "strip trailing slashes from operands")
	copyContents := fs.Bool("copy-contents", false, "copy contents of special files when recursive")
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
		rc:           rc,
		paths:        newPathResolver(rc),
		recursive:    *recursive || *recursiveUpper || *archive,
		preserve:     preserve,
		force:        *force,
		noClobber:    *noClobber,
		update:       *update,
		backup:       *backup != "",
		suffix:       *suffix,
		link:         *link,
		symlink:      *symlink,
		derefMode:    resolveDereferenceMode(args, *recursive || *recursiveUpper || *archive),
		attrsOnly:    *attrsOnly,
		copyContents: *copyContents,
		debug:        *debug,
		oneFS:        *oneFS,
		parents:      *parents,
		removeDest:   *removeDest,
		interactive:  *interactive,
		verbose:      *verbose,
		in:           inputReader(rc.In),
	}
	if c.recursive {
		c.umask = invocationUmask(rc)
	}
	defer c.paths.close()
	// GNU rule: of -i, -f, and -n, the one given last takes effect. Issue 7
	// defines -i and -f as independent steps, however: -i prompts before the
	// destination is opened, while -f is consulted only if that open fails.
	// POSIXLY_CORRECT therefore retains both when they are combined. The GNU
	// -n extension remains ordered against either option in both modes.
	posix := envPresent(rc.Env, "POSIXLY_CORRECT")
	switch lastOverride(args) {
	case 'f':
		c.noClobber = false
		if !posix {
			c.interactive = false
		}
	case 'n':
		c.force = false
		c.interactive = false
	case 'i':
		c.noClobber = false
		if !posix {
			c.force = false
		}
	}

	dest := ""
	srcs := operands
	if *targetDir != "" {
		dest = *targetDir
	} else {
		dest = operands[len(operands)-1]
		srcs = operands[:len(operands)-1]
	}
	di, err := os.Stat(c.path(dest))
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

func (c *copier) path(operand string) string { return c.paths.path(operand) }

// copyEntry dispatches one SOURCE operand. Without -r symlinks are
// followed (os.Stat); with -r they are copied as symlinks, per the
// GNU manual's -R default.
func (c *copier) copyEntry(src, dst string) {
	if src == "" {
		c.errf("cannot stat '': No such file or directory")
		return
	}
	stat := os.Stat
	if c.derefMode == dereferenceNone {
		stat = os.Lstat
	}
	fi, err := stat(c.path(src))
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
		// Lexical containment is not sufficient: target or one of its
		// ancestors can be a symbolic link back into source. Walk the existing
		// destination ancestors with Stat so aliases are resolved before any
		// directory is created. Otherwise the new destination becomes an entry
		// in the source traversal and cp recursively copies its own output.
		if destinationWithinSource(c.rc.Path(src), c.rc.Path(dst)) {
			c.errf("cannot copy a directory, '%s', into itself, '%s'", src, dst)
			return
		}
		if c.oneFS {
			if dev, ok := fileDev(fi); ok {
				c.rootDev = dev
				c.haveRootDev = true
			}
		}
		c.copyDir(src, dst, fi)
	case fi.Mode()&os.ModeSymlink != 0:
		c.copySymlink(src, dst, fi)
	case c.recursive && isSpecial(fi.Mode()) && !c.copyContents:
		c.copySpecial(src, dst, fi)
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
	for _, ancestor := range c.dirStack {
		if os.SameFile(ancestor, fi) {
			c.errf("cannot copy cyclic symbolic link '%s'", src)
			return
		}
	}
	c.dirStack = append(c.dirStack, fi)
	defer func() { c.dirStack = c.dirStack[:len(c.dirStack)-1] }()
	created := false
	// finalPerm is the mode a newly created directory receives after the
	// tree is populated when -p does not preserve the mode: the source
	// mode as modified by the invoking umask (POSIX cp step 2.d).
	finalPerm := fi.Mode().Perm() &^ c.umask
	if di, err := os.Lstat(c.path(dst)); err == nil {
		if !di.IsDir() {
			c.errf("cannot overwrite non-directory '%s' with directory '%s'", dst, src)
			return
		}
	} else {
		if !c.prepareParent(dst) {
			return
		}
		// POSIX: the directory is created with the source mode modified
		// by the umask, then OR'd with
		// S_IRWXU so children can land regardless of the source mode (the
		// GNU manual's read-only-source-dir behavior). The umask-filtered
		// mode is read back before widening so the final mode retains the
		// filtering.
		population := finalPerm | 0o700
		if c.preserve.ownership {
			// Ownership may change after population. Until then, do not
			// expose the incomplete tree to the source group or others.
			population = 0o700
		} else if c.preserve.mode {
			// Mode is restored after population; suppress group/other
			// write access while children are still being copied.
			population &^= 0o022
		}
		// Create at the restricted population mode from the outset. The
		// host umask can only remove bits, never expose extra ones; chmod
		// then establishes the exact invocation-owned result so a stricter
		// host mask cannot prevent recursive population.
		if err := os.Mkdir(c.path(dst), population); err != nil {
			c.errf("cannot create directory '%s': %s", dst, reason(err))
			return
		}
		if err := os.Chmod(c.path(dst), population); err != nil {
			c.errf("setting permissions for '%s': %s", dst, reason(err))
			return
		}
		created = true
	}
	c.verbosef("'%s' -> '%s'", src, dst)
	entries, err := os.ReadDir(c.path(src))
	if err != nil {
		c.errf("cannot access '%s': %s", src, reason(err))
	} else {
		for _, e := range entries {
			csrc := filepath.Join(src, e.Name())
			cdst := filepath.Join(dst, e.Name())
			ci, err := os.Lstat(c.path(csrc))
			if err != nil {
				c.errf("cannot stat '%s': %s", csrc, reason(err))
				continue
			}
			if c.derefMode == dereferenceAll && ci.Mode()&os.ModeSymlink != 0 {
				ci, err = os.Stat(c.path(csrc))
				if err != nil {
					c.errf("cannot stat '%s': %s", csrc, reason(err))
					continue
				}
			}
			switch {
			case ci.IsDir():
				c.copyDir(csrc, cdst, ci)
			case ci.Mode()&os.ModeSymlink != 0:
				c.copySymlink(csrc, cdst, ci)
			case isSpecial(ci.Mode()) && !c.copyContents:
				c.copySpecial(csrc, cdst, ci)
			default:
				c.copyFile(csrc, cdst, ci)
			}
		}
	}
	if c.preserve.any() {
		c.preserveAttrs(src, dst, fi)
	}
	if created && !c.preserve.mode {
		if err := os.Chmod(c.path(dst), finalPerm); err != nil {
			c.errf("setting permissions for '%s': %s", dst, reason(err))
		}
	}
}

type dereferenceMode uint8

const (
	dereferenceNone dereferenceMode = iota
	dereferenceCommandLine
	dereferenceAll
)

// resolveDereferenceMode applies the POSIX last-option rule for -H, -L, and
// -P. GNU -a/-d are physical-copy aliases and participate in the same ordering.
// Without an explicit mode, recursive copies are physical while non-recursive
// copies follow their command-line source.
func resolveDereferenceMode(args []string, recursive bool) dereferenceMode {
	mode := dereferenceAll
	if recursive {
		mode = dereferenceNone
	}
	for _, arg := range args {
		if arg == "--" {
			break
		}
		switch arg {
		case "--dereference":
			mode = dereferenceAll
			continue
		case "--dereference-command-line":
			mode = dereferenceCommandLine
			continue
		case "--no-dereference", "--no-dereference-preserve-links", "--archive":
			mode = dereferenceNone
			continue
		}
		if len(arg) < 2 || arg[0] != '-' || arg[1] == '-' {
			continue
		}
		for _, option := range arg[1:] {
			switch option {
			case 'H':
				mode = dereferenceCommandLine
			case 'L':
				mode = dereferenceAll
			case 'P', 'a', 'd':
				mode = dereferenceNone
			}
			// The rest of a short cluster is the value of these options,
			// not more option letters that can alter dereference mode.
			if option == 'S' || option == 't' || option == 'Z' {
				break
			}
		}
	}
	return mode
}

func (c *copier) copyFile(src, dst string, fi os.FileInfo) {
	sp, dp := c.path(src), c.path(dst)
	if _, err := os.Lstat(dp); err == nil {
		// POSIX step 1 precedes step 3's prompt: a source that is the
		// same file as the destination (or a destination directory that
		// a non-directory cannot replace) is diagnosed before overwrite
		// controls such as GNU -n are considered.
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
		if c.noClobber {
			return // -n: silently skip an otherwise valid overwrite
		}
		// GNU -u is an extension, but it must not suppress the required
		// same-file or type diagnostics above.
		if c.update && !sourceNewer(sp, dp) {
			return
		}
		if c.interactive && !c.confirm(dst) {
			// POSIX: a declined -i reply means "do nothing more with
			// source_file, go on to any remaining files" — a successful
			// skip, not an error, so the exit status is unaffected.
			return
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
	if !c.prepareParent(dst) {
		return
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
	out, err := c.rc.OpenFile(dp, flags, fi.Mode().Perm())
	if err != nil && c.force {
		// -f: if an existing destination file cannot be opened,
		// remove it and try again.
		if os.Remove(dp) == nil {
			out, err = c.rc.OpenFile(dp, flags, fi.Mode().Perm())
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
		rerr, werr := copyRegularData(out, in)
		_ = in.Close()
		cerr := out.Close()
		if rerr != nil {
			c.errf("error reading '%s': %s", src, reason(rerr))
			return
		}
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

// copyRegularData keeps read-side and write-side failures distinct. io.Copy
// returns one undifferentiated error, which previously caused a source read
// failure to be diagnosed as a destination write failure.
func copyRegularData(dst io.Writer, src io.Reader) (readErr, writeErr error) {
	buf := make([]byte, 32*1024)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			if werr == nil && written != n {
				werr = io.ErrShortWrite
			}
			if werr != nil {
				return nil, werr
			}
		}
		if rerr == io.EOF {
			return nil, nil
		}
		if rerr != nil {
			return rerr, nil
		}
		if n == 0 {
			return io.ErrNoProgress, nil
		}
	}
}

func (c *copier) copySymlink(src, dst string, fi os.FileInfo) {
	sp, dp := c.path(src), c.path(dst)
	target, err := os.Readlink(sp)
	if err != nil {
		c.errf("cannot read symbolic link '%s': %s", src, reason(err))
		return
	}
	if di, err := os.Lstat(dp); err == nil {
		// Under -P the symlink itself is the source file. Step 1 therefore
		// compares lstat identities, not the files referenced by the links.
		// This also covers two pathnames that are hard links to one symlink.
		if si, statErr := os.Lstat(sp); statErr == nil && os.SameFile(si, di) {
			c.errf("'%s' and '%s' are the same file", src, dst)
			return
		}
		// os.Remove removes empty directories, unlike the unlink operation
		// specified by cp. Classify this case before overwrite controls so a
		// physical symlink can never replace an existing destination directory.
		if di.IsDir() {
			c.errf("cannot overwrite directory '%s' with non-directory", dst)
			return
		}
		if c.noClobber {
			return
		}
		if c.update && !sourceNewer(sp, dp) {
			return
		}
		if c.interactive && !c.confirm(dst) {
			return // declined -i: successful skip, status unaffected
		}
		if c.backup && !c.backupDest(dst) {
			return
		}
		if err := os.Remove(dp); err != nil {
			c.errf("cannot remove '%s': %s", dst, reason(err))
			return
		}
	}
	if !c.prepareParent(dst) {
		return
	}
	if err := os.Symlink(target, dp); err != nil {
		c.errf("cannot create symbolic link '%s': %s", dst, reason(err))
		return
	}
	if c.preserve.any() {
		c.preserveSymlinkAttrs(dst, fi)
	}
	c.debugf("copied symbolic link '%s' -> '%s'", src, dst)
	c.verbosef("'%s' -> '%s'", src, dst)
}

// prepareParent creates destination ancestors only for the explicit GNU
// --parents extension. Plain POSIX cp must let the destination open/mkdir fail
// when its parent does not exist; silently inventing that hierarchy changes a
// failed copy into a successful one.
func (c *copier) prepareParent(dst string) bool {
	if !c.parents {
		return true
	}
	parent := filepath.Dir(dst)
	if parent == "." || parent == dst {
		return true
	}
	if err := os.MkdirAll(c.path(parent), 0o777); err != nil {
		c.errf("cannot create directory '%s': %s", parent, reason(err))
		return false
	}
	return true
}

// destinationWithinSource physically resolves source and the closest existing
// prefix of dst. Resolving only the source root's inode misses aliases to a
// subdirectory (alias -> source/sub), which are equally capable of feeding the
// newly created destination back into source traversal.
func destinationWithinSource(source, dst string) bool {
	sourcePath, err := filepath.EvalSymlinks(source)
	if err != nil {
		return false
	}
	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return false
	}
	dstPath, err := resolveExistingPrefix(dst)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(sourcePath, dstPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveExistingPrefix evaluates symlinks through the longest existing prefix
// and then reattaches missing trailing components. EvalSymlinks on the complete
// destination cannot do this because the destination normally does not exist.
func resolveExistingPrefix(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Abs(resolved)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		missing = append(missing, filepath.Base(path))
		path = parent
	}
}

// confirm writes the -i prompt to stderr and reads one reply line from
// the invocation's standard input. The affirmative match is the
// LC_MESSAGES yesexpr anchored at byte zero — "^[yY]" in the C/POSIX
// locale, plus j/J for the provisioned de_DE locale — so only the line
// terminator is stripped, never leading white space: " y" declines.
func (c *copier) confirm(dst string) bool {
	fmt.Fprintf(c.rc.Err, "cp: overwrite '%s'? ", dst)
	line, err := c.in.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	yes, matchErr := locale.MatchAffirmative(c.rc.Env, line)
	if matchErr != nil {
		c.errf("cannot interpret response: %s", matchErr)
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

func (c *copier) backupDest(dst string) bool {
	dp := c.path(dst)
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
	dp := c.path(dst)
	// Ownership is duplicated before the mode: several kernels clear
	// S_ISUID/S_ISGID as a side effect of chown() even for a no-op
	// change by the owner, so the bits survive only when chmod runs
	// after chown (POSIX leaves the duplication order unspecified).
	ownershipDuplicated := true
	if c.preserve.ownership {
		ownershipDuplicated = preserveOwner(dp, fi)
	}
	if c.preserve.mode {
		mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
		if !ownershipDuplicated {
			// POSIX -p: if the user ID or group ID cannot be duplicated,
			// the S_ISUID and S_ISGID bits shall be cleared.
			mode &^= os.ModeSetuid | os.ModeSetgid
		}
		if err := os.Chmod(dp, mode); err != nil {
			c.errf("preserving permissions for '%s': %s", dst, reason(err))
		}
	}
	if c.preserve.timestamps {
		access, ok := fileAtime(fi)
		if !ok {
			c.errf("preserving times for '%s': access time unsupported on this platform", dst)
		} else if err := os.Chtimes(dp, access, fi.ModTime()); err != nil {
			c.errf("preserving times for '%s': %s", dst, reason(err))
		}
	}
	_ = src
}

// preserveSymlinkAttrs is deliberately separate from preserveAttrs: chmod,
// chown, and Chtimes follow symbolic links on common platforms and would
// mutate the referent. Every operation here is either no-follow or a read-only
// comparison of the link inode.
func (c *copier) preserveSymlinkAttrs(dst string, fi os.FileInfo) {
	dp := c.path(dst)
	ownershipDuplicated := true
	if c.preserve.ownership {
		ownershipDuplicated = preserveLinkOwner(dp, fi)
	}
	if c.preserve.mode {
		want := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
		if !ownershipDuplicated {
			want &^= os.ModeSetuid | os.ModeSetgid
		}
		di, err := os.Lstat(dp)
		if err != nil {
			c.errf("preserving permissions for '%s': %s", dst, reason(err))
		} else if got := di.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky); got != want {
			// There is no portable no-follow chmod. Newly created symlinks have
			// the platform's fixed link mode; matching is success, differing
			// modes are an honest unsupported boundary.
			c.errf("preserving permissions for '%s': symbolic link mode unsupported on this platform", dst)
		}
	}
	if c.preserve.timestamps {
		access, ok := fileAtime(fi)
		if !ok {
			c.errf("preserving times for '%s': access time unsupported on this platform", dst)
		} else if err := preserveLinkTimes(dp, access, fi.ModTime()); err != nil {
			c.errf("preserving times for '%s': %s", dst, reason(err))
		}
	}
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

func envPresent(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
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
