package tool

import (
	"errors"
	"io"
	"os"
	"strings"
)

// IsClosedPipeError reports whether err is the normal result of a downstream
// pipeline segment exiting early. Windows can surface this as "file already
// closed" rather than a POSIX-style broken pipe.
func IsClosedPipeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrClosed) || errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "file already closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "pipe is being closed")
}
