//go:build !windows && !darwin

package unamecmd

import (
	"strings"

	"golang.org/x/sys/unix"
)

// probe fills sysinfo from uname(2). Kernel version strings can carry
// embedded newlines on some platforms (darwin); they collapse to
// spaces so the -a line stays one line, matching uname output.
func probe() (sysinfo, error) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return sysinfo{}, err
	}
	clean := func(b []byte) string {
		s := unix.ByteSliceToString(b)
		return strings.Join(strings.Fields(s), " ")
	}
	return sysinfo{
		sysname:          clean(u.Sysname[:]),
		nodename:         clean(u.Nodename[:]),
		release:          clean(u.Release[:]),
		version:          clean(u.Version[:]),
		machine:          clean(u.Machine[:]),
		processor:        "unknown",
		hardwarePlatform: "unknown",
	}, nil
}
