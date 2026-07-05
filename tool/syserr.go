package tool

import (
	"errors"
	"io/fs"
	"os"
	"unicode"
	"unicode/utf8"
)

// SysErrString renders err the way GNU coreutils does: the underlying OS errno
// message with its first letter capitalized. glibc/BSD strerror(3) capitalizes
// ("No such file or directory", "Permission denied", "Is a directory", …) while
// Go's syscall.Errno.Error() does not ("no such file or directory"). It unwraps
// *fs.PathError (== *os.PathError) so the text is just the errno, matching the
// GNU "<tool>: <name>: <errno>" shape where the caller already prints the name.
//
// Every command's error path should route through this (or SysErr) so error
// wording is byte-identical to GNU — verified by the perfbench fidelity matrix.
func SysErrString(err error) string {
	if err == nil {
		return ""
	}
	// Unwrap the os wrapper errors so only the errno text remains — the GNU
	// "<tool>: <name>: <errno>" shape, where the caller already prints the name.
	var pe *fs.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var le *os.LinkError
	if errors.As(err, &le) {
		err = le.Err
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		err = se.Err
	}
	return capitalizeFirst(err.Error())
}

// SysErr is SysErrString as an error value, for helpers/call sites that print
// the result with %v or return it up the stack.
func SysErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(SysErrString(err))
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	if !unicode.IsLower(r) {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}
