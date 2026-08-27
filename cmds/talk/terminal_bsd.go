//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package talkcmd

import "golang.org/x/sys/unix"

type controlChars struct{ erase, kill, intr, eof byte }

func enterTerminalMode(fd int) (controlChars, func() error, error) {
	original, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return controlChars{}, nil, err
	}
	controls := controlChars{original.Cc[unix.VERASE], original.Cc[unix.VKILL], original.Cc[unix.VINTR], original.Cc[unix.VEOF]}
	next := *original
	next.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.ISIG
	next.Cc[unix.VMIN] = 1
	next.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TIOCSETA, &next); err != nil {
		return controlChars{}, nil, err
	}
	return controls, func() error { return unix.IoctlSetTermios(fd, unix.TIOCSETA, original) }, nil
}
