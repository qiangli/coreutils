//go:build unix

package testcmd

import (
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/qiangli/coreutils/tool"
)

// accessOK answers -r/-w/-x by asking the kernel, not by reading mode
// bits: the GNU manual defines these primaries as "permission is
// granted", and mode bits alone cannot answer that — the effective user
// being root, an ACL, or a read-only mount all change the answer.
// AT_EACCESS selects the effective (not the real) user, which is the
// euidaccess semantics the manual describes.
//
// The plain path is tried first; only on failure, and only for a
// relative operand, is a directory-handle retry attempted (see
// openOperandDir) — a long-but-valid working directory joined with even
// a short operand can otherwise exceed the path-length limit as one
// materialized string, when the same lookup done relative to an
// already-open directory would succeed.
func accessOK(rc *tool.RunContext, operand string, op byte) (bool, error) {
	var mode uint32
	switch op {
	case 'r':
		mode = unix.R_OK
	case 'w':
		mode = unix.W_OK
	default:
		mode = unix.X_OK
	}
	if unix.Faccessat(unix.AT_FDCWD, rc.Path(operand), mode, unix.AT_EACCESS) == nil {
		return true, nil
	}
	if dir, ok := openOperandDir(rc, operand); ok {
		defer dir.Close()
		if unix.Faccessat(int(dir.Fd()), operand, mode, unix.AT_EACCESS) == nil {
			return true, nil
		}
	}
	return false, nil
}

// ownedByEffective answers -O (byGroup=false) and -G (byGroup=true).
// A file that cannot be stat'ed is simply not owned — that is false,
// not an error, exactly as for every other file primary.
func ownedByEffective(rc *tool.RunContext, operand string, byGroup bool) (bool, error) {
	st, err := statRaw(rc, operand)
	if err != nil {
		return false, nil
	}
	if byGroup {
		return int(st.Gid) == os.Getegid(), nil
	}
	return int(st.Uid) == os.Geteuid(), nil
}

// fileTimes returns the access and modification times for -N.
func fileTimes(rc *tool.RunContext, operand string) (atime, mtime time.Time, err error) {
	st, err := statRaw(rc, operand)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return time.Unix(st.Atim.Unix()), time.Unix(st.Mtim.Unix()), nil
}

// statRaw is statOperand for callers that need the raw unix.Stat_t
// (uid/gid/atime aren't exposed by os.FileInfo). See openOperandDir for
// why the directory-handle retry is needed at all.
func statRaw(rc *tool.RunContext, operand string) (unix.Stat_t, error) {
	var st unix.Stat_t
	err := unix.Stat(rc.Path(operand), &st)
	if err == nil {
		return st, nil
	}
	if dir, ok := openOperandDir(rc, operand); ok {
		defer dir.Close()
		if err2 := unix.Fstatat(int(dir.Fd()), operand, &st, 0); err2 == nil {
			return st, nil
		}
	}
	return st, err
}

// isTerminal reports whether f is a terminal. A window-size ioctl is
// the portable probe across every unix in scope: it succeeds on ttys
// and ptys and fails with ENOTTY on everything else.
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	return err == nil
}
