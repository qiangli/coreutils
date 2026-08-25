// Package mvcmd implements mv(1) per the GNU coreutils manual: rename
// SOURCE to DEST, or move SOURCE(s) to DIRECTORY. A rename that fails
// because source and destination are on different filesystems falls
// back to copy+remove, as GNU mv does.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/mv
// (BSD-3-Clause).
// Changes: rewired to the tool framework; added cross-device
// copy+remove fallback and GNU -f/-n/-v semantics.
package mvcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/qiangli/coreutils/pkg/locale"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/term"
)

var cmd = &tool.Tool{
	Name:     "mv",
	Synopsis: "Rename SOURCE to DEST, or move SOURCE(s) to DIRECTORY.",
	Usage:    "mv [OPTION]... SOURCE DEST\n   or: mv [OPTION]... SOURCE... DIRECTORY",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

type mover struct {
	rc          *tool.RunContext
	noClobber   bool
	update      bool
	interactive bool
	force       bool
	backup      bool
	backupMode  string
	suffix      string
	debug       bool
	verbose     bool
	failed      bool
	in          *bufio.Reader
	rawIn       io.Reader
	deps        moverDeps
}

type overwriteMode byte

const (
	overwriteDefault overwriteMode = iota
	overwriteForce
	overwriteNoClobber
	overwriteInteractive
	forceOption       = "-f"
	interactiveOption = "-i"
)

type overwriteValue struct {
	mode *overwriteMode
	set  overwriteMode
}

func (v overwriteValue) String() string { return "false" }
func (v overwriteValue) Type() string   { return "bool" }
func (v overwriteValue) Set(value string) error {
	if value != "true" {
		return fmt.Errorf("option does not take a value")
	}
	*v.mode = v.set
	return nil
}

type moverDeps struct {
	rename            func(string, string) error
	remove            func(string) error
	removeAll         func(string) error
	chmod             func(string, os.FileMode) error
	chtimes           func(string, time.Time, time.Time) error
	fchmod            func(*os.File, os.FileMode) error
	preserveOwner     func(string, os.FileInfo) error
	preserveFileOwner func(*os.File, os.FileInfo) error
	preserveFileTimes func(*os.File, os.FileInfo) error
	preserveLinkOwner func(string, os.FileInfo) error
	preserveLinkMode  func(string, os.FileInfo) error
	preserveLinkTimes func(string, os.FileInfo) error
	writable          func(string) bool
	terminal          func(io.Reader) bool
}

func defaultMoverDeps() moverDeps {
	return moverDeps{
		rename: rename, remove: os.Remove, removeAll: os.RemoveAll,
		chmod: os.Chmod, chtimes: os.Chtimes, preserveOwner: preserveOwner,
		fchmod:            func(f *os.File, mode os.FileMode) error { return f.Chmod(mode) },
		preserveFileOwner: preserveFileOwner, preserveFileTimes: preserveFileTimes,
		preserveLinkOwner: preserveLinkOwner, preserveLinkMode: preserveLinkMode,
		preserveLinkTimes: preserveLinkTimes,
		writable:          isWritable,
		terminal: func(r io.Reader) bool {
			f, ok := r.(interface{ Fd() uintptr })
			return ok && term.IsTerminal(int(f.Fd()))
		},
	}
}

func run(rc *tool.RunContext, args []string) int {
	return runWithDeps(rc, args, defaultMoverDeps())
}

func runWithDeps(rc *tool.RunContext, args []string, deps moverDeps) int {
	args = normalizeOptionalArgs(args)
	fs := tool.NewFlags(cmd.Name)
	overwrite := overwriteDefault
	forceValue := overwriteValue{mode: &overwrite, set: overwriteForce}
	noClobberValue := overwriteValue{mode: &overwrite, set: overwriteNoClobber}
	interactiveValue := overwriteValue{mode: &overwrite, set: overwriteInteractive}
	fs.VarP(forceValue, "force", forceOption[1:], "do not prompt before overwriting")
	fs.Lookup("force").NoOptDefVal = "true"
	fs.VarP(noClobberValue, "no-clobber", "n", "do not overwrite an existing file; silently skip it")
	fs.Lookup("no-clobber").NoOptDefVal = "true"
	fs.VarP(interactiveValue, "interactive", interactiveOption[1:], "prompt before overwrite")
	fs.Lookup("interactive").NoOptDefVal = "true"
	update := fs.BoolP("update", "u", false, "move only when SOURCE is newer than the destination or destination is missing")
	targetDir := fs.StringP("target-directory", "t", "", "move all SOURCE arguments into DIRECTORY")
	noTargetDir := fs.BoolP("no-target-directory", "T", false, "treat DEST as a normal file")
	backup := fs.StringP("backup", "b", "", "make a backup of each existing destination")
	fs.Lookup("backup").NoOptDefVal = "simple"
	suffix := fs.StringP("suffix", "S", "~", "override the usual backup suffix")
	debug := fs.Bool("debug", false, "explain move decisions on stderr")
	fs.Bool("strip-trailing-slashes", false, "strip trailing slashes from operands")
	fs.BoolP("progress", "g", false, "accepted for compatibility; progress output is a no-op")
	fs.StringP("context", "Z", "", "accepted for compatibility; SELinux context is a no-op")
	verbose := fs.BoolP("verbose", "v", false, "explain what is being done")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	operands = maybeStripTrailingSlashes(operands, fs.Changed("strip-trailing-slashes"))
	backupMode := *backup
	switch backupMode {
	case "", "none", "off", "simple", "existing", "nil", "never", "numbered", "t":
	default:
		return tool.UsageError(rc, cmd, "invalid --backup value '%s'", backupMode)
	}
	if *targetDir != "" && *noTargetDir {
		return tool.UsageError(rc, cmd, "cannot combine --target-directory and --no-target-directory")
	}
	switch len(operands) {
	case 0:
		return tool.UsageError(rc, cmd, "missing file operand")
	case 1:
		if *targetDir == "" {
			return tool.UsageError(rc, cmd, "missing destination file operand after '%s'", operands[0])
		}
	}

	m := &mover{
		rc: rc, noClobber: overwrite == overwriteNoClobber, update: *update,
		interactive: overwrite == overwriteInteractive, force: overwrite == overwriteForce,
		backup: backupMode != "" && backupMode != "nil", backupMode: backupMode, suffix: *suffix,
		debug: *debug, verbose: *verbose,
		in:    inputReader(rc.In),
		rawIn: rc.In,
		deps:  deps,
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
		fmt.Fprintf(rc.Err, "mv: target directory '%s' is not a directory\n", dest)
		return 1
	}
	if len(srcs) > 1 && !todir {
		fmt.Fprintf(rc.Err, "mv: target '%s' is not a directory\n", dest)
		return 1
	}
	// Validate the source before rejecting a slash-suffixed destination.
	// GNU mv diagnoses a missing source first. It also accepts a trailing
	// slash on a missing destination when the source is a directory, naming
	// the newly created directory without the slash.
	if len(dest) > 0 && os.IsPathSeparator(dest[len(dest)-1]) && !(err == nil && di.IsDir()) {
		normalizedDirDestination := false
		if len(srcs) == 1 {
			si, serr := os.Lstat(rc.Path(srcs[0]))
			if serr != nil {
				fmt.Fprintf(rc.Err, "mv: cannot stat '%s': %s\n", srcs[0], reason(serr))
				return 1
			}
			trimmed := dest
			for len(trimmed) > 1 && os.IsPathSeparator(trimmed[len(trimmed)-1]) {
				trimmed = trimmed[:len(trimmed)-1]
			}
			if _, trimmedErr := os.Lstat(rc.Path(trimmed)); os.IsNotExist(trimmedErr) && si.IsDir() {
				dest = trimmed
				normalizedDirDestination = true
			}
		}
		if !normalizedDirDestination {
			if err != nil {
				fmt.Fprintf(rc.Err, "mv: cannot move '%s' to '%s': %s\n", srcs[0], dest, reason(err))
				return 1
			}
			fmt.Fprintf(rc.Err, "mv: cannot move '%s' to '%s': Not a directory\n", srcs[0], dest)
			return 1
		}
	}
	for _, src := range srcs {
		dst := dest
		if todir {
			dst = filepath.Join(dest, filepath.Base(src))
		}
		m.move(src, dst)
	}
	if m.failed {
		return 1
	}
	return 0
}

func (m *mover) move(src, dst string) {
	if src == "" {
		m.errf("cannot stat '': No such file or directory")
		return
	}
	sp, dp := m.rc.Path(src), m.rc.Path(dst)
	if _, err := os.Lstat(sp); err != nil {
		m.errf("cannot stat '%s': %s", src, reason(err))
		return
	}
	if m.noClobber {
		if _, err := os.Lstat(dp); err == nil {
			return // -n: silently skip; exit status unaffected
		}
	}
	if _, err := os.Lstat(dp); err == nil {
		if m.update && !sourceNewer(sp, dp) {
			return
		}
		if !m.force {
			if m.interactive {
				if !m.confirm(dst, false) {
					return
				}
			} else if m.deps.terminal(m.rawIn) {
				if !m.deps.writable(dp) {
					if !m.confirm(dst, true) {
						return
					}
				}
			}
		}
		// POSIX Issue 7 orders confirmation before same-file resolution.
		if si, e1 := os.Stat(sp); e1 == nil {
			if dsi, e2 := os.Stat(dp); e2 == nil && os.SameFile(si, dsi) {
				m.errf("'%s' and '%s' are the same file", src, dst)
				return
			}
		}
		if m.backup && !m.backupDest(dst) {
			return
		}
	}
	err := m.deps.rename(sp, dp)
	if err == nil {
		m.debugf("renamed '%s' -> '%s'", src, dst)
		m.verbosef("renamed '%s' -> '%s'", src, dst)
		return
	}
	if isCrossDevice(err) {
		if m.copyMove(src, dst) {
			m.debugf("copied across file systems and removed '%s'", src)
			m.verbosef("renamed '%s' -> '%s'", src, dst)
		}
		return
	}
	m.errf("cannot move '%s' to '%s': %s", src, dst, reason(err))
}

func (m *mover) confirm(dst string, unwritable bool) bool {
	if unwritable {
		fmt.Fprintf(m.rc.Err, "mv: replace '%s', overriding mode? ", dst)
	} else {
		fmt.Fprintf(m.rc.Err, "mv: overwrite '%s'? ", dst)
	}
	line, err := m.in.ReadString('\n')
	if err != nil && line == "" {
		return false
	}
	yes, matchErr := locale.MatchAffirmative(m.rc.Env, line)
	if matchErr != nil {
		m.errf("cannot interpret response: %s", matchErr)
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

func (m *mover) backupDest(dst string) bool {
	dp := m.rc.Path(dst)
	mode := m.backupMode
	if mode == "none" || mode == "off" || mode == "nil" {
		return true
	}
	if mode == "" {
		mode = "existing"
	}
	if mode == "existing" {
		if hasNumberedBackup(dp) {
			mode = "numbered"
		} else {
			mode = "simple"
		}
	}
	bp := dp + m.suffix
	if mode == "numbered" || mode == "t" {
		n, err := nextNumberedBackup(dp)
		if err != nil {
			m.errf("cannot backup '%s': %s", dst, reason(err))
			return false
		}
		bp = fmt.Sprintf("%s.~%d~", dp, n)
	} else {
		if err := m.deps.remove(bp); err != nil && !os.IsNotExist(err) {
			m.errf("cannot backup '%s': %s", dst, reason(err))
			return false
		}
	}
	if err := m.deps.rename(dp, bp); err != nil {
		m.errf("cannot backup '%s': %s", dst, reason(err))
		return false
	}
	return true
}

func hasNumberedBackup(path string) bool {
	n, err := nextNumberedBackup(path)
	return err == nil && n > 1
}

func nextNumberedBackup(path string) (int, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	max := 0
	prefix := base + ".~"
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, "~") {
			continue
		}
		n, err := strconv.Atoi(name[len(prefix) : len(name)-1])
		if err == nil && n > max {
			max = n
		}
	}
	return max + 1, nil
}

func sourceNewer(src, dst string) bool {
	si, serr := os.Stat(src)
	di, derr := os.Stat(dst)
	if serr != nil || derr != nil {
		return true
	}
	return si.ModTime().After(di.ModTime())
}

// copyMove follows the POSIX Issue 7 EXDEV order: reject type mismatches,
// remove an existing destination, duplicate a fresh hierarchy, then remove
// the source. Characteristic failures are diagnosed by copyNode but are not
// hierarchy failures and therefore do not prevent source removal.
func (m *mover) copyMove(src, dst string) bool {
	sp, dp := m.rc.Path(src), m.rc.Path(dst)
	si, err := os.Lstat(sp)
	if err != nil {
		m.errf("cannot stat '%s': %s", src, reason(err))
		return false
	}
	if di, err := os.Lstat(dp); err == nil {
		if si.IsDir() != di.IsDir() {
			if si.IsDir() {
				m.errf("cannot overwrite non-directory '%s' with directory '%s'", dst, src)
			} else {
				m.errf("cannot overwrite directory '%s' with non-directory '%s'", dst, src)
			}
			return false
		}
		// POSIX step 5 removes the destination path itself (unlink/rmdir).
		// It must not recursively erase a non-empty destination hierarchy.
		if err := m.deps.remove(dp); err != nil {
			m.errf("cannot remove '%s': %s", dst, reason(err))
			return false
		}
	} else if !os.IsNotExist(err) {
		m.errf("cannot access '%s': %s", dst, reason(err))
		return false
	}
	if !m.copyNode(src, dst) {
		return false
	}
	if err := m.deps.removeAll(m.rc.Path(src)); err != nil {
		m.errf("cannot remove '%s': %s", src, reason(err))
		return false
	}
	return true
}

func (m *mover) copyNode(src, dst string) bool {
	sp, dp := m.rc.Path(src), m.rc.Path(dst)
	fi, err := os.Lstat(sp)
	if err != nil {
		m.errf("cannot stat '%s': %s", src, reason(err))
		return false
	}
	switch {
	case fi.IsDir():
		if err := os.Mkdir(dp, fi.Mode().Perm()|0o700); err != nil {
			m.errf("cannot create directory '%s': %s", dst, reason(err))
			return false
		}
		entries, err := os.ReadDir(sp)
		if err != nil {
			m.errf("cannot access '%s': %s", src, reason(err))
			return false
		}
		ok := true
		for _, e := range entries {
			if !m.copyNode(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())) {
				ok = false
			}
		}
		m.preserveAttrs(dst, dp, fi)
		return ok
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(sp)
		if err != nil {
			m.errf("cannot read symbolic link '%s': %s", src, reason(err))
			return false
		}
		if err := os.Symlink(target, dp); err != nil {
			m.errf("cannot create symbolic link '%s': %s", dst, reason(err))
			return false
		}
		m.preserveSymlinkAttrs(dst, dp, fi)
		return true
	case fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0:
		err = copySpecialNode(dp, fi)
		if err != nil {
			m.errf("cannot create special file '%s': %s", dst, reason(err))
			return false
		}
		m.preserveAttrs(dst, dp, fi)
		return true
	default:
		in, err := os.Open(sp)
		if err != nil {
			m.errf("cannot open '%s' for reading: %s", src, reason(err))
			return false
		}
		defer in.Close()
		// O_EXCL prevents a destination symlink introduced after the top-level
		// removal, or during recursive duplication, from being followed.
		out, err := os.OpenFile(dp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fi.Mode().Perm())
		if err != nil {
			m.errf("cannot create regular file '%s': %s", dst, reason(err))
			return false
		}
		_, werr := io.Copy(out, in)
		if werr != nil {
			_ = out.Close()
			m.errf("error writing '%s': %s", dst, reason(werr))
			return false
		}
		m.preserveRegularAttrs(dst, out, fi)
		if err := out.Close(); err != nil {
			m.errf("error writing '%s': %s", dst, reason(err))
			return false
		}
		return true
	}
}

func (m *mover) preserveAttrs(dst, dp string, fi os.FileInfo) {
	err := m.deps.preserveOwner(dp, fi)
	mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err != nil {
		mode &^= (os.ModeSetuid | os.ModeSetgid)
		m.warnf("preserving ownership for '%s': %s", dst, reason(err))
	}
	if err := m.deps.chmod(dp, mode); err != nil {
		m.warnf("preserving permissions for '%s': %s", dst, reason(err))
	}
	if err := m.deps.chtimes(dp, atime(fi), fi.ModTime()); err != nil {
		m.warnf("preserving times for '%s': %s", dst, reason(err))
	}
}

func (m *mover) preserveRegularAttrs(dst string, out *os.File, fi os.FileInfo) {
	mode := fi.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if err := m.deps.preserveFileOwner(out, fi); err != nil {
		mode &^= (os.ModeSetuid | os.ModeSetgid)
		m.warnf("preserving ownership for '%s': %s", dst, reason(err))
	}
	if err := m.deps.fchmod(out, mode); err != nil {
		m.warnf("preserving permissions for '%s': %s", dst, reason(err))
	}
	if err := m.deps.preserveFileTimes(out, fi); err != nil {
		m.warnf("preserving times for '%s': %s", dst, reason(err))
	}
}

func (m *mover) preserveSymlinkAttrs(dst, dp string, fi os.FileInfo) {
	if err := m.deps.preserveLinkOwner(dp, fi); err != nil {
		m.warnf("preserving symbolic link ownership for '%s': %s", dst, reason(err))
	}
	if err := m.deps.preserveLinkMode(dp, fi); err != nil {
		m.warnf("preserving symbolic link permissions for '%s': %s", dst, reason(err))
	}
	if err := m.deps.preserveLinkTimes(dp, fi); err != nil {
		m.warnf("preserving symbolic link times for '%s': %s", dst, reason(err))
	}
}

func (m *mover) errf(format string, a ...any) {
	fmt.Fprintf(m.rc.Err, "mv: "+format+"\n", a...)
	m.failed = true
}

func (m *mover) warnf(format string, a ...any) {
	fmt.Fprintf(m.rc.Err, "mv: "+format+"\n", a...)
}

func (m *mover) verbosef(format string, a ...any) {
	if m.verbose {
		fmt.Fprintf(m.rc.Out, format+"\n", a...)
	}
}

func (m *mover) debugf(format string, a ...any) {
	if m.debug {
		fmt.Fprintf(m.rc.Err, "mv: debug: "+format+"\n", a...)
	}
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
			out[i] = "--backup=existing"
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

// reason unwraps err to its root cause and capitalizes the first
// letter, matching the strerror() shape GNU diagnostics use.
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
