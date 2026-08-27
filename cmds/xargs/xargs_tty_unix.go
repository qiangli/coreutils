//go:build !windows

package xargscmd

import (
	"io"
	"os"
)

// defaultTTYOpener reads -p's yes/no prompt reply from the controlling
// terminal (POSIX: "The user is asked whether to execute the command").
func defaultTTYOpener() (io.ReadCloser, error) {
	return os.Open("/dev/tty")
}
