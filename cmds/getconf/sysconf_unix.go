//go:build unix

package getconfcmd

import (
	"os"
	"runtime"
	"strconv"

	"golang.org/x/sys/unix"
)

// golang.org/x/sys/unix exports Sysconf and Pathconf but NOT the _SC_/_PC_
// selector constants, and those numbers differ between Linux and Darwin.
// Hard-coding remembered numbers is how a build silently reports the wrong
// limit, so system values are derived from portable sources wherever one exists
// (the stdlib, getrlimit, sysctl) and the per-OS selectors are confined to
// pathconf_*.go, which sysconf_test.go cross-checks against the host's own
// getconf.

func sysconfStr(which int) (string, bool) {
	switch which {
	case scPagesize:
		return strconv.Itoa(os.Getpagesize()), true
	case scNprocessorsConf, scNprocessorsOnln:
		return strconv.Itoa(runtime.NumCPU()), true
	case scClkTck:
		return clockTicksStr()
	case scOpenMax:
		return rlimitStr(unix.RLIMIT_NOFILE)
	case scArgMax:
		return argMaxStr()
	case scChildMax:
		return childMaxStr()
	case scNgroupsMax:
		return ngroupsMaxStr()
	}
	return undefined, true
}

func rlimitStr(res int) (string, bool) {
	var rl unix.Rlimit
	if err := unix.Getrlimit(res, &rl); err != nil {
		return undefined, true
	}
	if rl.Cur == ^uint64(0) {
		return undefined, true
	}
	return strconv.FormatUint(uint64(rl.Cur), 10), true
}
