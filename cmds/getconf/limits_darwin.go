//go:build darwin

package getconfcmd

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// Darwin publishes these through sysctl rather than deriving them from rlimits.
func argMaxStr() (string, bool) {
	if v, err := unix.SysctlUint32("kern.argmax"); err == nil && v > 0 {
		return strconv.FormatUint(uint64(v), 10), true
	}
	return undefined, true
}

func childMaxStr() (string, bool) {
	if v, err := unix.SysctlUint32("kern.maxprocperuid"); err == nil && v > 0 {
		return strconv.FormatUint(uint64(v), 10), true
	}
	return rlimitStr(unix.RLIMIT_NPROC)
}

func ngroupsMaxStr() (string, bool) {
	if v, err := unix.SysctlUint32("kern.ngroups"); err == nil && v > 0 {
		return strconv.FormatUint(uint64(v), 10), true
	}
	return "16", true
}

func symloopMaxStr() (string, bool) { return "32", true }
func clockTicksStr() (string, bool) { return "100", true }
