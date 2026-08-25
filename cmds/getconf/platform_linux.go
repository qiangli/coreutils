//go:build linux

package getconfcmd

import (
	"os"
	"runtime"
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
	case "_POSIX_V6_LP64_OFF64", "_POSIX_V7_LP64_OFF64":
		if linuxLP64Build() {
			return "1", true
		}
		return undefined, true
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

// The Issue 7 -v contract applies when the corresponding _POSIX_V* query is
// neither -1 nor undefined. Linux uses LP64 with 64-bit off_t on these Go
// targets. We intentionally do not infer an ILP32 or LPBIG C environment.
func platformSpecification(specification string) bool {
	if !linuxLP64Build() {
		return false
	}
	return specification == "POSIX_V6_LP64_OFF64" || specification == "POSIX_V7_LP64_OFF64"
}

func linuxLP64Build() bool {
	switch runtime.GOARCH {
	case "amd64", "arm64", "loong64", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x":
		return true
	}
	return false
}

func platformConfstrValue(name string) (string, bool) {
	return undefined, isConfstrName(name)
}

func platformDifferentialNames() []string { return nil }
