//go:build linux

package ddcmd

import (
	"os"

	"golang.org/x/sys/unix"
)

// Linux latches POLLHUP when a FIFO writer opens and closes before the first
// read. Opening non-blocking is therefore cancellable without weakening the
// blocking-open state machine implemented by interruptReader.
func interruptibleOpenNamedFIFORead(path string, sigctx *interruptContext) (*os.File, error) {
	if sigctx.Interrupted() {
		return nil, errInterrupted
	}
	for {
		fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == unix.EINTR {
			if sigctx.Interrupted() {
				return nil, errInterrupted
			}
			continue
		}
		if err != nil {
			return nil, &os.PathError{Op: "open", Path: path, Err: err}
		}
		fd = moveDescriptor(fd)
		return os.NewFile(uintptr(fd), path), nil
	}
}
