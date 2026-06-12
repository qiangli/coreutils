//go:build unix

package mvcmd

import (
	"errors"
	"syscall"
)

// isCrossDevice reports whether a rename failed because source and
// destination are on different filesystems (EXDEV), which triggers
// GNU mv's copy+remove fallback.
func isCrossDevice(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}
