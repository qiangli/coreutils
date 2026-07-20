//go:build darwin

package ttycmd

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// ttyName reports the terminal path for f, or ok=false when f is not
// a terminal. Darwin has no /proc; the device node is found by scanning
// /dev — the same approach ttyname(3) takes when it cannot use the
// kernel devname() path.
//
// Every character device in /dev is a candidate. The scan is NOT
// limited to names beginning with "tty": real terminals such as
// /dev/console and serial callout devices (/dev/cu.*) would otherwise
// be missed and misreported as "not a tty" even though the termios
// ioctl already confirmed the fd is a terminal. Symlinks (/dev/stdin,
// /dev/fd/*) are skipped via Lstat so they cannot resolve to the
// terminal and shadow the actual device node.
//
// Both rdev and inode must match. rdev alone is insufficient on Darwin:
// several special devices (/dev/console, /dev/fbt, /dev/lockstat,
// /dev/profile, …) share rdev 0, so rdev matching alone could return
// the wrong node. The inode uniquely identifies the device node that
// fstat saw on the descriptor.
func ttyName(f *os.File) (string, bool) {
	fd := int(f.Fd())
	if _, err := unix.IoctlGetTermios(fd, unix.TIOCGETA); err != nil {
		return "", false
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return "", false
	}
	entries, err := os.ReadDir("/dev")
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		path := "/dev/" + e.Name()
		fi, err := os.Lstat(path)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		sys, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		if uint64(sys.Rdev) == uint64(st.Rdev) && sys.Ino == st.Ino {
			return path, true
		}
	}
	return "", false
}
