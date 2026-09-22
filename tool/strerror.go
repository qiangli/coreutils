package tool

import (
	"errors"
	"syscall"
)

// A Windows system call reports its failure as a Win32 status whose
// Error() is the OS's own English sentence — "The system cannot find the
// file specified." — while every GNU diagnostic this repository reproduces
// is the POSIX strerror(3) text, "No such file or directory". The agent
// contract says an applet's output does not vary with the host, and bash's
// own fixtures diff those sentences (heredoc.tests line 133 wants exactly
// `grep: *.c: No such file or directory`), so the errno text is normalized
// here for every applet that formats a path error.
//
// bashy's interpreter carries the same table (mvdan.cc/sh/v3's
// interp.posixErrorText) for the errors IT reports; that helper is
// unexported and interp is not a dependency of this module, so the table is
// duplicated rather than shared. Keep the two in sync: a row added there
// belongs here too.
//
// Only the errnos GNU coreutils and grep actually print are mapped. An
// unmapped Windows error keeps its native sentence — a wrong POSIX name
// would be worse than an honest one. Unix messages are untouched: strerror
// is already what the OS returns there.
func posixErrorTextMode(err error, windows bool) (string, bool) {
	if !windows {
		return "", false
	}
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return "", false
	}
	switch uintptr(errno) {
	case 2, 3, 123: // FILE_NOT_FOUND, PATH_NOT_FOUND, INVALID_NAME
		return "No such file or directory", true
	case 5: // ACCESS_DENIED
		return "Permission denied", true
	case 80, 183: // FILE_EXISTS, ALREADY_EXISTS
		return "File exists", true
	case 145: // DIR_NOT_EMPTY
		return "Directory not empty", true
	case 267: // DIRECTORY
		return "Not a directory", true
	case 193: // BAD_EXE_FORMAT
		return "Exec format error", true
	case 32: // SHARING_VIOLATION
		return "Device or resource busy", true
	case 109, 232: // BROKEN_PIPE, NO_DATA
		return "Broken pipe", true
	case 206: // FILENAME_EXCED_RANGE
		return "File name too long", true
	default:
		return "", false
	}
}
