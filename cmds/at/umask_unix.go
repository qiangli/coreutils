//go:build unix

package atcmd

import "syscall"

// processUmask snapshots the inherited mask for a standalone at invocation.
// Embedded shells provide their virtual mask through RunContext and never use
// this process-global fallback.
func processUmask() (uint32, bool) {
	mask := syscall.Umask(0)
	syscall.Umask(mask)
	return uint32(mask), true
}
