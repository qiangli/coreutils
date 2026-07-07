//go:build !windows

package nicecmd

import "golang.org/x/sys/unix"

func currentNice() int {
	n, err := unix.Getpriority(unix.PRIO_PROCESS, 0)
	if err != nil {
		return 0
	}
	return n
}

func setNice(n int) error {
	return unix.Setpriority(unix.PRIO_PROCESS, 0, n)
}
