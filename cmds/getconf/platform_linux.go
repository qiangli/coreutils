//go:build linux

package getconfcmd

import (
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Linux does not expose libc's sysconf/confstr table through a kernel API.
// Do not mistake Go's target ABI for a glibc (or musl) conformance statement.
func platformValue(name string) (string, bool) {
	switch name {
	case "BC_BASE_MAX", "BC_STRING_MAX", "INT_MAX", "LINE_MAX", "RE_DUP_MAX", "SYMLOOP_MAX", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION":
		return undefined, true
	case "SIGQUEUE_MAX":
		return rlimitStr(unix.RLIMIT_SIGPENDING)
	case "_NPROCESSORS_CONF":
		return linuxCPUSetCount("/sys/devices/system/cpu/possible"), true
	case "_NPROCESSORS_ONLN":
		return linuxCPUSetCount("/sys/devices/system/cpu/online"), true
	}
	return "", false
}

func linuxCPUSetCount(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return undefined
	}
	count := uint64(0)
	for _, field := range strings.Split(strings.TrimSpace(string(data)), ",") {
		bounds := strings.Split(field, "-")
		if len(bounds) < 1 || len(bounds) > 2 || bounds[0] == "" {
			return undefined
		}
		first, err := strconv.ParseUint(bounds[0], 10, 32)
		if err != nil {
			return undefined
		}
		last := first
		if len(bounds) == 2 {
			last, err = strconv.ParseUint(bounds[1], 10, 32)
			if err != nil || last < first {
				return undefined
			}
		}
		count += last - first + 1
	}
	if count == 0 {
		return undefined
	}
	return strconv.FormatUint(count, 10)
}

func platformSpecification(string) bool { return false }

func platformConfstrValue(name string) (string, bool) {
	return undefined, isConfstrName(name)
}

func platformDifferentialNames() []string { return nil }
