//go:build !windows

package meet

import "syscall"

// processAlive reports whether a pid is still running on THIS host.
//
// Signal 0 performs the permission and existence checks without delivering
// anything. EPERM means the process exists but belongs to another user — still
// alive, so still the lock's owner.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
