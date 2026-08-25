//go:build darwin

package pathchkcmd

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	pcNameMax = 4
	pcPathMax = 5
)

func filesystemLimits(dir string) (int, int, error) {
	// pathconf reports an indeterminate variable (no limit) by returning -1
	// with errno unchanged, which reaches Go as a negative value and a nil
	// error. That is a valid answer, not a failure: the caller skips the
	// corresponding length check (limitLookup's non-positive convention).
	pathMax, err := unix.Pathconf(dir, pcPathMax)
	if err != nil {
		return 0, 0, fmt.Errorf("PATH_MAX: %w", err)
	}
	nameMax, err := unix.Pathconf(dir, pcNameMax)
	if err != nil {
		return 0, 0, fmt.Errorf("NAME_MAX: %w", err)
	}
	return pathMax, nameMax, nil
}
