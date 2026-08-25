//go:build darwin

package xargscmd

import "golang.org/x/sys/unix"

func sysArgMax() int {
	if value, err := unix.SysctlUint32("kern.argmax"); err == nil && value > 0 {
		return int(value)
	}
	return 131072
}
