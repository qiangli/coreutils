//go:build windows

package tool

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func installOwnedFrame(c *exec.Cmd, f *os.File) (uint64, func(), error) {
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, nil, err
	}
	var dup windows.Handle
	var dupErr error
	if err := rc.Control(func(fd uintptr) {
		self := windows.CurrentProcess()
		dupErr = windows.DuplicateHandle(self, windows.Handle(fd), self, &dup, 0, true, windows.DUPLICATE_SAME_ACCESS)
	}); err != nil {
		return 0, nil, err
	}
	if dupErr != nil {
		return 0, nil, dupErr
	}
	if c.SysProcAttr == nil {
		c.SysProcAttr = &syscall.SysProcAttr{}
	}
	c.SysProcAttr.AdditionalInheritedHandles = append(c.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(dup))
	return uint64(dup), func() { _ = windows.CloseHandle(dup) }, nil
}
