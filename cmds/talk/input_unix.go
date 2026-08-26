//go:build unix

package talkcmd

import (
	"context"
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

type polledTerminalInput struct{ file *os.File }

func newTerminalInput(r io.Reader) (terminalInput, error) {
	if f, ok := r.(interface{ Fd() uintptr }); ok {
		fd, err := unix.Dup(int(f.Fd()))
		if err != nil {
			return nil, err
		}
		owned := os.NewFile(uintptr(fd), "talk-terminal-input")
		if owned == nil {
			_ = unix.Close(fd)
			return nil, errors.New("cannot own duplicated terminal input")
		}
		return &polledTerminalInput{file: owned}, nil
	}
	return newAsyncInput(r)
}

func (p *polledTerminalInput) Poll(ctx context.Context, wait time.Duration) (inputEvent, bool) {
	select {
	case <-ctx.Done():
		return inputEvent{}, false
	default:
	}
	ms := int(wait.Milliseconds())
	if ms < 1 {
		ms = 1
	}
	fds := []unix.PollFd{{Fd: int32(p.file.Fd()), Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR}}
	n, err := unix.Poll(fds, ms)
	if err == unix.EINTR || n == 0 {
		return inputEvent{}, false
	}
	if err != nil {
		return inputEvent{err: err}, true
	}
	buf := make([]byte, 4096)
	n, err = unix.Read(int(p.file.Fd()), buf)
	if n > 0 {
		return inputEvent{data: buf[:n]}, true
	}
	if err != nil {
		return inputEvent{err: err}, true
	}
	return inputEvent{err: io.EOF}, true
}

func (p *polledTerminalInput) Close() error { return p.file.Close() }
