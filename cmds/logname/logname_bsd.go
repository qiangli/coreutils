//go:build darwin || dragonfly || freebsd

package lognamecmd

import (
	"bytes"
	"golang.org/x/sys/unix"
	"unsafe"
)

func platformLoginName() string {
	buf := make([]byte, 256)
	_, _, err := unix.Syscall(unix.SYS_GETLOGIN, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if err != 0 {
		return ""
	}
	i := bytes.IndexByte(buf, 0)
	if i >= 0 {
		buf = buf[:i]
	}
	if len(buf) == 0 {
		return ""
	}
	return bareUser(string(buf))
}
