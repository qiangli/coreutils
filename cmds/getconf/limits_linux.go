//go:build linux

package getconfcmd

import "golang.org/x/sys/unix"

// Linux does not expose the complete exec argument accounting rule to Go.  In
// particular it includes the current environment and architecture-dependent
// page limits, so an rlimit-only answer is not sysconf(ARG_MAX).
func argMaxStr() (string, bool) {
	return undefined, true
}

func childMaxStr() (string, bool) { return rlimitStr(unix.RLIMIT_NPROC) }

func ngroupsMaxStr() (string, bool) {
	return undefined, true
}

func reDupMaxStr() (string, bool)   { return undefined, true }
func symloopMaxStr() (string, bool) { return undefined, true }
func clockTicksStr() (string, bool) { return undefined, true }
