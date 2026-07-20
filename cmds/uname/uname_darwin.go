//go:build darwin

package unamecmd

import (
	"strings"

	"golang.org/x/sys/unix"
)

// probe fills sysinfo from uname(2) plus Darwin sysctl data for the
// GNU-only processor and hardware-platform fields.
func probe() (sysinfo, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return sysinfo{}, err
	}
	clean := func(b []byte) string {
		s := unix.ByteSliceToString(b)
		return strings.Join(strings.Fields(s), " ")
	}

	info := sysinfo{
		sysname:   clean(u.Sysname[:]),
		nodename:  clean(u.Nodename[:]),
		release:   clean(u.Release[:]),
		version:   clean(u.Version[:]),
		machine:   clean(u.Machine[:]),
		processor: "unknown",
	}

	if model, err := unix.Sysctl("hw.model"); err == nil {
		info.hardwarePlatform = strings.Join(strings.Fields(model), " ")
	} else {
		info.hardwarePlatform = "unknown"
	}

	if cputype, err := unix.SysctlUint32("hw.cputype"); err == nil {
		info.processor = darwinProcessor(cputype)
	}

	return info, nil
}

func darwinProcessor(cputype uint32) string {
	switch cputype & 0x00ffffff {
	case 7:
		return "i386"
	case 12:
		return "arm"
	default:
		return "unknown"
	}
}
