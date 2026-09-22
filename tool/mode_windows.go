//go:build windows

package tool

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procWaitNamedPipeW = windows.NewLazySystemDLL("kernel32.dll").NewProc("WaitNamedPipeW")

// lstat keeps os.Lstat for filesystem objects. A Windows named-pipe path must
// be recognized first: os.Lstat can report a live pipe as an ordinary regular
// file, losing its type, or report it absent. An os.Open would create a client
// connection and steal a process-substitution rendezvous. WaitNamedPipeW
// checks the named-pipe namespace without opening a client connection.
func lstat(path string) (os.FileInfo, error) {
	name, ok := namedPipeName(path)
	if !ok {
		return os.Lstat(path)
	}
	// The fixture spells this //./pipe/name, but WaitNamedPipeW requires the
	// native \\.\pipe\name form. Never pass the slash spelling through to Win32.
	path16, convErr := windows.UTF16PtrFromString(`\\.\pipe\` + name)
	if convErr != nil {
		return nil, &os.PathError{Op: "lstat", Path: path, Err: convErr}
	}
	// Zero asks for the server's default wait, which need not be short. One
	// millisecond is enough to distinguish an absent pipe from a busy one:
	// ERROR_SEM_TIMEOUT still proves an instance exists.
	r1, _, waitErr := procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(path16)), 1)
	if r1 != 0 || waitErr == windows.ERROR_SEM_TIMEOUT || waitErr == windows.ERROR_PIPE_BUSY {
		return namedPipeInfo{name: name}, nil
	}
	return nil, &os.PathError{Op: "lstat", Path: path, Err: waitErr}
}
