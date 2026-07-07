//go:build unix

package nicecmd

import "golang.org/x/sys/unix"

func currentPriority() int {
	n, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
	if err != nil {
		return 0
	}
	return n
}

func setPriority(n int) error {
	return unix.Setpriority(unix.PRIO_PROCESS, 0, n)
}
