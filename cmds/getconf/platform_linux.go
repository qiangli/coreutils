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
	case "INT_MAX", "SYMLOOP_MAX", "_POSIX_VERSION", "_POSIX2_VERSION", "_XOPEN_VERSION":
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

// standardUtilsPath is the stable standard-utility search path this product
// guarantees when the shell locates the standard utilities without $PATH —
// the exact contract `command -p` resolves against: the conventional system
// directories in their usual order, with the multicall bin directory first.
// POSIX defines getconf PATH as "the value of PATH used to find standard
// utilities", so getconf is the queryable form of that guarantee and must
// report it rather than undefined. Linux exposes no kernel API for libc's
// confstr(_CS_PATH) policy, but this value is a product guarantee, not a host
// measurement: it deliberately diverges from a host glibc's "/bin:/usr/bin".
const standardUtilsPath = "/opt/bashy/bin:/bin:/usr/bin:/sbin:/usr/sbin"

func platformConfstrValue(name string) (string, bool) {
	if name == "PATH" {
		return standardUtilsPath, true
	}
	if linuxLP64Build() {
		switch name {
		case "POSIX_V6_LP64_OFF64_CFLAGS", "POSIX_V6_LP64_OFF64_LDFLAGS", "POSIX_V6_LP64_OFF64_LIBS",
			"POSIX_V7_LP64_OFF64_CFLAGS", "POSIX_V7_LP64_OFF64_LDFLAGS", "POSIX_V7_LP64_OFF64_LIBS",
			"POSIX_V7_THREADS_CFLAGS", "POSIX_V7_THREADS_LDFLAGS":
			// No special compiler or linker options are required for the
			// native LP64 environment. confstr represents that as an empty
			// string, which is distinct from an undefined environment.
			return "", true
		case "POSIX_V6_WIDTH_RESTRICTED_ENVS":
			return "POSIX_V6_LP64_OFF64", true
		case "POSIX_V7_WIDTH_RESTRICTED_ENVS":
			return "POSIX_V7_LP64_OFF64", true
		case "V6_ENV", "V7_ENV":
			// This product requires no extra environment assignments to
			// select its conforming environment. An empty confstr value is
			// defined and can safely be expanded before an env invocation.
			return "", true
		}
	}
	return undefined, isConfstrName(name)
}

func platformDifferentialNames() []string { return nil }
