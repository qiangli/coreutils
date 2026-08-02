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
func accessOK(_ *tool.RunContext, path string, op byte) (bool, error) {
	var mode uint32
	switch op {
	case 'r':
		mode = unix.R_OK
	case 'w':
		mode = unix.W_OK
	default:
		mode = unix.X_OK
	}
	return unix.Faccessat(unix.AT_FDCWD, path, mode, unix.AT_EACCESS) == nil, nil
}

// ownedByEffective answers -O (byGroup=false) and -G (byGroup=true).
// A file that cannot be stat'ed is simply not owned — that is false,
// not an error, exactly as for every other file primary.
func ownedByEffective(path string, byGroup bool) (bool, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return false, nil
	}
	if byGroup {
		return int(st.Gid) == os.Getegid(), nil
	}
	return int(st.Uid) == os.Geteuid(), nil
}

// fileTimes returns the access and modification times for -N.
func fileTimes(path string) (atime, mtime time.Time, err error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return time.Time{}, time.Time{}, err
	}
	return time.Unix(st.Atim.Unix()), time.Unix(st.Mtim.Unix()), nil
}

// isTerminal reports whether f is a terminal. A window-size ioctl is
// the portable probe across every unix in scope: it succeeds on ttys
// and ptys and fails with ENOTTY on everything else.
func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	return err == nil
}
