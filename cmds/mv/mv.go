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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
	"unicode"

	"github.com/qiangli/coreutils/tool"
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
	rc        *tool.RunContext
	noClobber bool
	verbose   bool
	failed    bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("force", "f", false, "do not prompt before overwriting (this implementation never prompts)")
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

	m := &mover{rc: rc, noClobber: *noClobber, verbose: *verbose}
	// GNU rule: of -f and -n, the one given last takes effect.
	if lastOverride(args) == 'f' {
		m.noClobber = false
	}

	dest := operands[len(operands)-1]
	srcs := operands[:len(operands)-1]
	di, err := os.Stat(rc.Path(dest))
	todir := err == nil && di.IsDir()
	if len(srcs) > 1 && !todir {
		fmt.Fprintf(rc.Err, "mv: target '%s' is not a directory\n", dest)
		return 1
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
	if si, e1 := os.Stat(sp); e1 == nil {
		if dsi, e2 := os.Stat(dp); e2 == nil && os.SameFile(si, dsi) {
			m.errf("'%s' and '%s' are the same file", src, dst)
			return
		}
	}
	err := os.Rename(sp, dp)
	if err == nil {
		m.verbosef("renamed '%s' -> '%s'", src, dst)
		return
	}
	if isCrossDevice(err) {
		if m.copyMove(src, dst) {
			m.verbosef("renamed '%s' -> '%s'", src, dst)
		}
		return
	}
	m.errf("cannot move '%s' to '%s': %s", src, dst, reason(err))
}

// copyMove is the cross-filesystem fallback: copy the tree (mode and
// mtime preserved; symlinks copied as symlinks), then remove the
// source. The source is left in place if any part of the copy fails.
func (m *mover) copyMove(src, dst string) bool {
	if !m.copyNode(src, dst) {
		return false
	}
	if err := os.RemoveAll(m.rc.Path(src)); err != nil {
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
		if di, err := os.Lstat(dp); err == nil {
			if !di.IsDir() {
				m.errf("cannot overwrite non-directory '%s' with directory '%s'", dst, src)
				return false
			}
		} else if err := os.Mkdir(dp, fi.Mode().Perm()|0o700); err != nil {
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
		_ = os.Chmod(dp, fi.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky))
		_ = os.Chtimes(dp, time.Time{}, fi.ModTime())
		return ok
	case fi.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(sp)
		if err != nil {
			m.errf("cannot read symbolic link '%s': %s", src, reason(err))
			return false
		}
		if _, err := os.Lstat(dp); err == nil {
			if err := os.Remove(dp); err != nil {
				m.errf("cannot remove '%s': %s", dst, reason(err))
				return false
			}
		}
		if err := os.Symlink(target, dp); err != nil {
			m.errf("cannot create symbolic link '%s': %s", dst, reason(err))
			return false
		}
		return true
	default:
		in, err := os.Open(sp)
		if err != nil {
			m.errf("cannot open '%s' for reading: %s", src, reason(err))
			return false
		}
		defer in.Close()
		out, err := os.OpenFile(dp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
		if err != nil {
			m.errf("cannot create regular file '%s': %s", dst, reason(err))
			return false
		}
		_, werr := io.Copy(out, in)
		cerr := out.Close()
		if werr != nil || cerr != nil {
			if werr == nil {
				werr = cerr
			}
			m.errf("error writing '%s': %s", dst, reason(werr))
			return false
		}
		_ = os.Chmod(dp, fi.Mode()&(os.ModePerm|os.ModeSetuid|os.ModeSetgid|os.ModeSticky))
		_ = os.Chtimes(dp, time.Time{}, fi.ModTime())
		return true
	}
}

func (m *mover) errf(format string, a ...any) {
	fmt.Fprintf(m.rc.Err, "mv: "+format+"\n", a...)
	m.failed = true
}

func (m *mover) verbosef(format string, a ...any) {
	if m.verbose {
		fmt.Fprintf(m.rc.Out, format+"\n", a...)
	}
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
