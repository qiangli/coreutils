//go:build windows

package ttycmd

import (
	"os"

	"golang.org/x/sys/windows"
)

// ttyName reports whether f is a console handle. Windows has no
// per-terminal device path; the console device name "CON" is
// reported, mirroring what GNU tty builds print in Windows
// environments.
func ttyName(f *os.File) (string, bool) {
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(f.Fd()), &mode); err != nil {
		return "", false
	}
	return "CON", true
}
