//go:build linux

package whocmd

import (
	"fmt"
	"os"
	"strings"
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
	if _, err := unix.IoctlGetTermios(fd, unix.TCGETS); err != nil {
		return "", false
	}
	name, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil || !strings.HasPrefix(name, "/dev/") {
		return "", false
	}
	return name, true
}

func accessTime(path string) (time.Time, bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return time.Time{}, false
	}
	return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
}
