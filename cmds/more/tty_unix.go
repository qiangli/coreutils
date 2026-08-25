//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package morecmd

import (
	"context"
	"io"
	"os"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

var openControllingTTY = func(_ *tool.RunContext) (*ttyChannel, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	fd := int(f.Fd())
	return &ttyChannel{
		fd:    fd,
		hasFd: true,
		close: f.Close,
		readCommand: func(ctx context.Context) (command byte, readErr error) {
			if ctx == nil {
				ctx = context.Background()
			}
			state, err := term.MakeRaw(fd)
			if err != nil {
				return 0, err
			}
			defer func() {
				if err := term.Restore(fd, state); readErr == nil && err != nil {
					readErr = err
				}
			}()
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			for {
				if err := ctx.Err(); err != nil {
					return 0, err
				}
				n, err := unix.Poll(fds, 50)
				if err != nil {
					if err == unix.EINTR {
						continue
					}
					return 0, err
				}
				if n == 0 {
					continue
				}
				var b [1]byte
				n, err = unix.Read(fd, b[:])
				if n == 1 {
					return b[0], nil
				}
				if err != nil {
					return 0, err
				}
				return 0, io.EOF
			}
		},
	}, nil
}
