//go:build windows

package rmdircmd

import (
	"errors"
	"syscall"
)

// ERROR_DIR_NOT_EMPTY (145): what os.Remove actually returns on Windows for
// a non-empty directory — errors.Is against the POSIX ENOTEMPTY does not
// match it, so --ignore-fail-on-non-empty needs the native code too.
const errorDirNotEmpty = syscall.Errno(145)

func isNonEmptySys(err error) bool {
	return errors.Is(err, errorDirNotEmpty)
}
