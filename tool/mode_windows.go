//go:build windows

package tool

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procWaitNamedPipeW = windows.NewLazySystemDLL("kernel32.dll").NewProc("WaitNamedPipeW")

// lstat keeps os.Lstat for filesystem objects. Windows named pipes are not
// filesystem objects: os.Lstat consequently reports them absent, while an
// os.Open would create a client connection and steal a process-substitution
// rendezvous. WaitNamedPipeW answers whether the object exists without opening
// it; a busy pipe still exists, so it receives the same synthetic stat result.
func lstat(path string) (os.FileInfo, error) {
	fi, err := os.Lstat(path)
	if err == nil {
		return fi, nil
	}
	name, ok := namedPipeName(path)
	if !ok {
		return fi, err
	}
	// The fixture spells this //./pipe/name, but WaitNamedPipeW requires the
	// native \\.\pipe\name form. Never pass the slash spelling through to Win32.
	path16, convErr := windows.UTF16PtrFromString(`\\.\pipe\` + name)
	if convErr != nil {
		return fi, err
	}
	// Zero asks for the server's default wait, which need not be short. One
	// millisecond is enough to distinguish an absent pipe from a busy one:
	// ERROR_SEM_TIMEOUT still proves an instance exists.
	r1, _, waitErr := procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(path16)), 1)
	if r1 != 0 || waitErr == windows.ERROR_SEM_TIMEOUT || waitErr == windows.ERROR_PIPE_BUSY {
		return namedPipeInfo{name: name}, nil
	}
	return fi, err
}
