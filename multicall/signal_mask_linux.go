//go:build linux

package multicall

import (
	"runtime"
	"syscall"
	"unsafe"
)

// kernelSigaction is the rt_sigaction ABI shared by the Linux architectures
// in the shipping matrix (amd64/arm64).
type kernelSigaction struct {
	handler  uintptr
	flags    uintptr
	restorer uintptr
	mask     uint64
}

func restoreDefaultAndUnblock(sig syscall.Signal) error {
	runtime.LockOSThread()
	if err := setLinuxSignalHandler(sig, 0); err != nil {
		return err
	}
	mask := uint64(1) << (uint(sig) - 1)
	const sigUnblock = 1
	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_RT_SIGPROCMASK,
		sigUnblock,
		uintptr(unsafe.Pointer(&mask)),
		0,
		8, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

func setLinuxSignalHandler(sig syscall.Signal, handler uintptr) error {
	action := kernelSigaction{handler: handler}
	_, _, errno := syscall.RawSyscall6(
		syscall.SYS_RT_SIGACTION,
		uintptr(sig),
		uintptr(unsafe.Pointer(&action)),
		0,
		8, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
