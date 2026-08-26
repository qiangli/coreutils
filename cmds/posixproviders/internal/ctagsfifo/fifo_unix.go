//go:build unix

package ctagsfifo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func openFIFO(ctx context.Context, path string, original os.FileInfo, wait bool) (*os.File, error) {
	for {
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err == nil {
			f := os.NewFile(uintptr(fd), path)
			current, statErr := f.Stat()
			if statErr != nil || current.Mode()&os.ModeNamedPipe == 0 || !os.SameFile(original, current) {
				_ = f.Close()
				if statErr != nil {
					return nil, statErr
				}
				return nil, fmt.Errorf("output FIFO changed during ctags execution")
			}
			return f, nil
		}
		if !wait || !errors.Is(err, unix.ENXIO) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func openPrivateOutput(path string, original os.FileInfo) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("private output changed during ctags execution: %w", err)
	}
	f := os.NewFile(uintptr(fd), path)
	current, statErr := f.Stat()
	if statErr != nil || !current.Mode().IsRegular() || !os.SameFile(original, current) {
		_ = f.Close()
		if statErr != nil {
			return nil, statErr
		}
		return nil, fmt.Errorf("private output changed during ctags execution")
	}
	return f, nil
}

// copyPrivateOutput keeps the FIFO descriptor nonblocking and waits in short
// poll slices whenever its buffer fills. This makes cancellation deterministic
// even after a reader has rendezvoused but stopped consuming: no goroutine can
// remain trapped in a blocking write(2).
func copyPrivateOutput(ctx context.Context, out, in *os.File) error {
	buf := make([]byte, 32*1024)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if err := writeFIFO(ctx, int(out.Fd()), buf[:n]); err != nil {
				return err
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
}

func writeFIFO(ctx context.Context, fd int, data []byte) error {
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := unix.Write(fd, data)
		if n > 0 {
			data = data[n:]
		}
		switch {
		case err == nil && n > 0:
			continue
		case err == nil:
			return io.ErrShortWrite
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK):
			if err := waitFIFOReady(ctx, fd); err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func waitFIFOReady(ctx context.Context, fd int) error {
	const pollSlice = 10 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		wait := pollSlice
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return ctx.Err()
			}
			if remaining < wait {
				wait = remaining
			}
		}
		milliseconds := int((wait + time.Millisecond - 1) / time.Millisecond)
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
		_, err := unix.Poll(fds, milliseconds)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		// A timeout is also a retry point. In particular, Darwin does not
		// reliably surface a closed FIFO reader as a poll event; retrying write
		// obtains EPIPE instead of waiting forever for readiness that cannot come.
		return nil
	}
}
