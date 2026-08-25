//go:build windows

package unamecmd

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// probe fills sysinfo from RtlGetVersion (registry-free, immune to
// compatibility-mode lies) plus os.Hostname and the GOARCH mapping.
// POSIX requires -v to produce an implementation-defined version string.
// Windows does not expose a Unix-style kernel-version field, so the build
// number from RtlGetVersion is the version string for this implementation.
func probe() (sysinfo, error) {
	host, err := os.Hostname()
	if err != nil {
		return sysinfo{}, err
	}
	v := windows.RtlGetVersion()
	release := fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
	return sysinfo{
		sysname:          "Windows_NT",
		nodename:         host,
		release:          release,
		version:          fmt.Sprintf("Build %d", v.BuildNumber),
		machine:          gnuArch(),
		processor:        "unknown",
		hardwarePlatform: "unknown",
	}, nil
}
