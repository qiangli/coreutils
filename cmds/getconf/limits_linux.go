//go:build linux

package getconfcmd

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// ARG_MAX on Linux is a quarter of the stack rlimit, floored at the historic
// value — the kernel's own rule (fs/exec.c), not an approximation.
func argMaxStr() (string, bool) {
	var rl unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_STACK, &rl); err != nil {
		return undefined, true
	}
	v := int64(rl.Cur / 4)
	if v < 131072 {
		v = 131072
	}
	return strconv.FormatInt(v, 10), true
}

func childMaxStr() (string, bool) { return rlimitStr(unix.RLIMIT_NPROC) }

func ngroupsMaxStr() (string, bool) {
	if b, err := os.ReadFile("/proc/sys/kernel/ngroups_max"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s, true
		}
	}
	return "65536", true
}
