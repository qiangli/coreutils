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
		// Windows: ERROR_NO_DATA (232) and ERROR_BROKEN_PIPE (109) — a downstream
		// segment (e.g. `head`) closed the read end. Both are normal early-exit.
		strings.Contains(msg, "pipe is being closed") ||
		strings.Contains(msg, "pipe has been ended")
}
