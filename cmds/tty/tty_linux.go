//go:build linux

package ttycmd

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// ttyName reports the terminal path for f, or ok=false when f is not
// a terminal. The fd is this process's own, so /proc/self/fd resolves
// it directly (the moral equivalent of ttyname(3)).
func ttyName(f *os.File) (string, bool) {
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
