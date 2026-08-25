//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package newgrpcmd

import "syscall"

func maximumSupplementaryGroups() int {
	limit, err := syscall.SysctlUint32("kern.ngroups")
	if err != nil || limit == 0 {
		return 0
	}
	return int(limit)
}
