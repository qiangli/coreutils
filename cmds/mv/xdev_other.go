//go:build !unix && !windows

package mvcmd

// isCrossDevice always reports false on platforms without a known
// cross-device rename errno; the plain rename error is surfaced.
func isCrossDevice(err error) bool { return false }
