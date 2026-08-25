//go:build unix

package prcmd

import (
	"io"
	"os"
)

// defaultOpenControlTTY opens the controlling terminal that -p/-f wait on
// for the required carriage return, per POSIX.
func defaultOpenControlTTY() (io.ReadCloser, error) {
	return os.OpenFile("/dev/tty", os.O_RDONLY, 0)
}
