//go:build linux

package xargscmd

import "golang.org/x/sys/unix"

func sysArgMax() int {
	const (
		minimum = uint64(131072)
		maximum = uint64(6 * 1024 * 1024)
	)
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_STACK, &limit); err != nil {
		return int(minimum)
	}
	value := limit.Cur / 4
	if value < minimum {
		value = minimum
	}
	if value > maximum {
		value = maximum
	}
	return int(value)
}
