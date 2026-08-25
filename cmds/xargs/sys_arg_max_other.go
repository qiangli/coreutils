//go:build !darwin && !linux

package xargscmd

// POSIX requires ARG_MAX to be at least 4096. Use the widely supported
// 128-KiB floor on platforms without a pure-Go runtime query.
func sysArgMax() int {
	return 131072
}
