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
func ttyName(f *os.File) (string, bool, error) {
	fd := int(f.Fd())
	if _, err := unix.IoctlGetTermios(fd, unix.TCGETS); err != nil {
		return "", false, nil
	}
	name, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
	if err != nil {
		return "", false, err
	}
	if !strings.HasPrefix(name, "/dev/") {
		return "", false, fmt.Errorf("terminal pathname %q is not a device pathname", name)
	}
	return name, true, nil
}
