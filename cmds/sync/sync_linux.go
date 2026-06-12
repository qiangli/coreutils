//go:build linux

package synccmd

import "syscall"

// syncAll commits all cached writes system-wide. On Linux the
// syscall has no return value.
func syncAll() error {
	syscall.Sync()
	return nil
}
