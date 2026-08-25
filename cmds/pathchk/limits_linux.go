//go:build linux

package pathchkcmd

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func filesystemLimits(dir string) (int, int, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, 0, err
	}
	if st.Namelen <= 0 {
		return 0, 0, fmt.Errorf("filesystem reported no NAME_MAX")
	}
	// Linux's pathname-copy ABI rejects a pathname buffer of 4096 bytes,
	// independently of the mounted filesystem; statfs supplies the genuinely
	// filesystem-specific component limit.
	return 4096, int(st.Namelen), nil
}
