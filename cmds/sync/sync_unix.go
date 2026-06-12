//go:build unix && !linux

package synccmd

import "syscall"

// syncAll commits all cached writes system-wide. On the BSD-derived
// platforms (incl. darwin) the syscall reports an error.
func syncAll() error {
	return syscall.Sync()
}
