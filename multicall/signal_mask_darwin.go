//go:build darwin

package multicall

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Darwin's userspace sigaction layout is handler, mask, flags. Use the raw
// kernel boundary because os/signal.Reset deliberately preserves parts of the
// Go runtime's signal machinery and cannot provide execve-equivalent SIG_DFL.
type darwinSigaction struct {
	handler uintptr
	mask    uint32
	flags   int32
}

func restoreDefaultAndUnblock(sig syscall.Signal) error {
	runtime.LockOSThread()
	var action darwinSigaction // zero handler is SIG_DFL
	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_SIGACTION,
		uintptr(sig),
		uintptr(unsafe.Pointer(&action)),
		0, 0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	mask := uint32(1) << (uint(sig) - 1)
	const sigUnblock = 2
	_, _, errno = syscall.RawSyscall6(
		syscall.SYS___PTHREAD_SIGMASK,
		sigUnblock,
		uintptr(unsafe.Pointer(&mask)),
		0, 0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
