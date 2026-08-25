//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package ddcmd

import "golang.org/x/sys/unix"

type pollEvents = int16

const (
	pollIn  pollEvents = unix.POLLIN
	pollOut pollEvents = unix.POLLOUT
	pollHup pollEvents = unix.POLLHUP
)

func moveDescriptor(fd int) int {
	if fd >= 0 && fd < 10 {
		nfd, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 10)
		if err == nil {
			_ = unix.Close(fd)
			return nfd
		}
	}
	return fd
}

func duplicateDescriptor(fd int) (int, error) {
	return unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 10)
}
