//go:build darwin

package whocmd

import (
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func stdinTTY(rc *tool.RunContext) (string, bool) {
	f, ok := rc.In.(*os.File)
	if !ok {
		return "", false
	}
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

func accessTime(path string) (time.Time, bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return time.Time{}, false
	}
	return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
}
