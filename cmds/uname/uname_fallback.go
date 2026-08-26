//go:build !unix && !windows

package unamecmd

import (
	"os"
	"runtime"
)

// probe supplies implementation-defined symbols on targets without uname(2).
// The Go target and runtime version are preferable to pretending that a Unix
// kernel provider exists; hostname lookup failures remain observable errors.
func probe() (sysinfo, error) {
	host, err := os.Hostname()
	if err != nil {
		return sysinfo{}, err
	}
	return sysinfo{
		sysname:          runtime.GOOS,
		nodename:         host,
		release:          runtime.Version(),
		version:          runtime.Version(),
		machine:          gnuArch(),
		processor:        "unknown",
		hardwarePlatform: "unknown",
	}, nil
}
