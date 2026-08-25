//go:build windows

package prcmd

import (
	"errors"
	"io"
)

// Windows has no /dev/tty equivalent this tool can open. -p/-f can still be
// requested; the controlling terminal is simply unavailable, so the pause
// fails closed with a clear diagnostic instead of guessing at a console
// handle that would not honor the POSIX wait semantics.
func defaultOpenControlTTY() (io.ReadCloser, error) {
	return nil, errors.New("controlling terminal (/dev/tty) is not available on windows")
}
