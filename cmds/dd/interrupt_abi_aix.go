//go:build aix

package ddcmd

import "golang.org/x/sys/unix"

// AIX declares PollFd.Events and Revents as uint16. The other supported Unix
// targets use int16, so keep the ABI-specific type behind this build boundary.
type pollEvents = uint16

const (
	pollIn  pollEvents = unix.POLLIN
	pollOut pollEvents = unix.POLLOUT
	pollHup pollEvents = unix.POLLHUP
)

func moveDescriptor(fd int) int {
	if fd < 0 || fd >= 10 {
		return fd
	}
	// AIX has F_DUPFD but not F_DUPFD_CLOEXEC. Set FD_CLOEXEC before exposing
	// the duplicate to the rest of dd; the command does not spawn children in
	// this interval.
	nfd, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD, 10)
	if err != nil {
		return fd
	}
	_, _ = unix.FcntlInt(uintptr(nfd), unix.F_SETFD, unix.FD_CLOEXEC)
	_ = unix.Close(fd)
	return nfd
}

func duplicateDescriptor(fd int) (int, error) {
	nfd, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD, 10)
	if err != nil {
		return -1, err
	}
	if _, err := unix.FcntlInt(uintptr(nfd), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		_ = unix.Close(nfd)
		return -1, err
	}
	return nfd, nil
}
