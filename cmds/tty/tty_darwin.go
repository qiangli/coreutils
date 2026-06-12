//go:build darwin

package ttycmd

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ttyName reports the terminal path for f, or ok=false when f is not
// a terminal. Darwin has no /proc; the device node is found by
// matching the descriptor's rdev against /dev/tty* entries (the same
// scan ttyname(3) performs over /dev).
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
		if !strings.HasPrefix(e.Name(), "tty") {
			continue
		}
		path := "/dev/" + e.Name()
		fi, err := os.Stat(path)
		if err != nil {
			continue
		}
		sys, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			continue
		}
		if uint64(sys.Rdev) == uint64(st.Rdev) && fi.Mode()&os.ModeCharDevice != 0 {
			return path, true
		}
	}
	return "", false
}
