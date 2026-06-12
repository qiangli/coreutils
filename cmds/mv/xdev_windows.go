//go:build windows

package mvcmd

import (
	"errors"
	"syscall"
)

// errorNotSameDevice is windows' ERROR_NOT_SAME_DEVICE (0x11), what
// MoveFileEx returns for a cross-volume rename.
const errorNotSameDevice = syscall.Errno(0x11)

// isCrossDevice reports whether a rename failed because source and
// destination are on different volumes, which triggers GNU mv's
// copy+remove fallback.
func isCrossDevice(err error) bool {
	return errors.Is(err, errorNotSameDevice) || errors.Is(err, syscall.EXDEV)
}
