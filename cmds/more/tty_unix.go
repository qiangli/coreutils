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

var openControllingTTY = func(rc *tool.RunContext) (*ttyChannel, error) {
	if f, ok := rc.Err.(*os.File); ok {
		fd := int(f.Fd())
		if flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0); err == nil && flags&unix.O_ACCMODE != unix.O_WRONLY {
			return fileTTYChannel(f, false), nil
		}
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return fileTTYChannel(f, true), nil
}

func fileTTYChannel(f *os.File, owned bool) *ttyChannel {
	fd := int(f.Fd())
	raw := term.IsTerminal(fd)
	closeFn := func() error { return nil }
	if owned {
		closeFn = f.Close
	}
	ch := &ttyChannel{
		fd:    fd,
		hasFd: true,
		close: closeFn,
		readCommand: func(ctx context.Context) (command byte, readErr error) {
			if ctx == nil {
				ctx = context.Background()
			}
			if !raw {
				var b [1]byte
				n, err := f.Read(b[:])
				if n == 1 {
					return b[0], nil
				}
				if err != nil {
					return 0, err
				}
				return 0, io.EOF
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
	}
	if raw {
		ch.editorIO = f
	}
	return ch
}
