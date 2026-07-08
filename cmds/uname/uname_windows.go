//go:build windows

package unamecmd

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// probe fills sysinfo from RtlGetVersion (registry-free, immune to
// compatibility-mode lies) plus os.Hostname and the GOARCH mapping.
// There is no kernel-version string equivalent; version stays empty
// and -a omits the field.
func probe() (sysinfo, error) {
	host, err := os.Hostname()
	if err != nil {
		return sysinfo{}, err
	}
	v := windows.RtlGetVersion()
	return sysinfo{
		sysname:          "Windows_NT",
		nodename:         host,
		release:          fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber),
		version:          "",
		machine:          gnuArch(),
		processor:        "unknown",
		hardwarePlatform: "unknown",
	}, nil
}
