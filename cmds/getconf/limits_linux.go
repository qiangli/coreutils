//go:build linux

package getconfcmd

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// Linux does not expose the complete exec argument accounting rule to Go.  In
// particular it includes the current environment and architecture-dependent
// page limits, so an rlimit-only answer is not sysconf(ARG_MAX).
func argMaxStr() (string, bool) {
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_STACK, &limit); err != nil {
		return "131072", true
	}
	value := limit.Cur / 4
	if value < 131072 {
		value = 131072
	}
	if value > 6*1024*1024 {
		value = 6 * 1024 * 1024
	}
	return strconv.FormatUint(value, 10), true
}

func childMaxStr() (string, bool) { return rlimitStr(unix.RLIMIT_NPROC) }

func ngroupsMaxStr() (string, bool) {
	data, err := os.ReadFile("/proc/sys/kernel/ngroups_max")
	if err != nil {
		return undefined, true
	}
	value := strings.TrimSpace(string(data))
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return undefined, true
	}
	return value, true
}

func symloopMaxStr() (string, bool) { return undefined, true }
func clockTicksStr() (string, bool) {
	data, err := os.ReadFile("/proc/self/auxv")
	if err != nil {
		return undefined, true
	}
	word := strconv.IntSize / 8
	for off := 0; off+2*word <= len(data); off += 2 * word {
		var tag, value uint64
		if word == 8 {
			tag = binary.NativeEndian.Uint64(data[off:])
			value = binary.NativeEndian.Uint64(data[off+word:])
		} else {
			tag = uint64(binary.NativeEndian.Uint32(data[off:]))
			value = uint64(binary.NativeEndian.Uint32(data[off+word:]))
		}
		if tag == 17 && value > 0 { // AT_CLKTCK from the kernel auxiliary vector.
			return strconv.FormatUint(value, 10), true
		}
	}
	return undefined, true
}
