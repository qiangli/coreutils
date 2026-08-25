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
	pathMax, err := unix.Pathconf(dir, pcPathMax)
	if err != nil {
		return 0, 0, fmt.Errorf("PATH_MAX: %w", err)
	}
	if pathMax <= 0 {
		return 0, 0, fmt.Errorf("filesystem reported no PATH_MAX")
	}
	nameMax, err := unix.Pathconf(dir, pcNameMax)
	if err != nil {
		return 0, 0, fmt.Errorf("NAME_MAX: %w", err)
	}
	if nameMax <= 0 {
		return 0, 0, fmt.Errorf("filesystem reported no NAME_MAX")
	}
	return pathMax, nameMax, nil
}
