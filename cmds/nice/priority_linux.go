//go:build linux

package nicecmd

import "golang.org/x/sys/unix"

func currentPriority() int {
	// Linux's getpriority syscall returns 20 minus the nice value. The libc
	// wrapper normally performs this conversion, but x/sys/unix exposes the
	// raw syscall result.
	n, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
	if err != nil {
		return 0
	}
	return nzero - n
}

func setPriority(pid, n int) error {
	return unix.Setpriority(unix.PRIO_PROCESS, pid, n)
}
