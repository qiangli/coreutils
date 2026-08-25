//go:build linux

package pathchkcmd

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func filesystemLimits(dir string) (int, int, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs: %w", err)
	}
	// Linux's pathname-copy ABI rejects a pathname buffer of 4096 bytes,
	// independently of the mounted filesystem; statfs supplies the genuinely
	// filesystem-specific component limit. A zero f_namelen means the
	// filesystem reports no component limit (limitLookup's non-positive
	// convention skips the check) rather than a query failure.
	return 4096, int(st.Namelen), nil
}
