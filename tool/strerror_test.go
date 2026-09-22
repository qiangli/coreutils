package tool

import (
	"io/fs"
	"syscall"
	"testing"
)

// TestPosixErrorTextMode is the Story #682 regression for the second half of
// the Windows fixture gap: bash's heredoc.tests diffs `grep: *.c: No such
// file or directory`, and the runner printed the OS's own sentence instead.
func TestPosixErrorTextMode(t *testing.T) {
	cases := []struct {
		errno uintptr
		want  string
	}{
		{2, "No such file or directory"},
		{3, "No such file or directory"},
		{123, "No such file or directory"},
		{5, "Permission denied"},
		{80, "File exists"},
		{183, "File exists"},
		{145, "Directory not empty"},
		{267, "Not a directory"},
		{193, "Exec format error"},
		{32, "Device or resource busy"},
		{109, "Broken pipe"},
		{232, "Broken pipe"},
		{206, "File name too long"},
	}
	for _, tc := range cases {
		got, ok := posixErrorTextMode(syscall.Errno(tc.errno), true)
		if !ok || got != tc.want {
			t.Errorf("posixErrorTextMode(%d) = %q,%v; want %q,true", tc.errno, got, ok, tc.want)
		}
		// The wrappers an applet actually holds must unwrap to the same text.
		wrapped := &fs.PathError{Op: "open", Path: "x", Err: syscall.Errno(tc.errno)}
		if got, ok := posixErrorTextMode(wrapped, true); !ok || got != tc.want {
			t.Errorf("posixErrorTextMode(PathError %d) = %q,%v; want %q,true", tc.errno, got, ok, tc.want)
		}
	}
}

// An unmapped Windows status keeps its native sentence: inventing a POSIX
// name for an errno GNU never prints would be a worse diagnostic than an
// honest one.
func TestPosixErrorTextModeUnmapped(t *testing.T) {
	if got, ok := posixErrorTextMode(syscall.Errno(1234), true); ok {
		t.Errorf("posixErrorTextMode(1234) = %q,true; want no mapping", got)
	}
	// Off windows mode nothing is rewritten, whatever the host.
	if got, ok := posixErrorTextMode(syscall.Errno(2), false); ok {
		t.Errorf("posixErrorTextMode(2, unix) = %q,true; want no mapping", got)
	}
}

// SysErrString keeps the Unix wording byte for byte: the errno text there is
// already strerror(3), and the capitalization rule is the only change.
func TestSysErrStringUnixWordingUnchanged(t *testing.T) {
	err := &fs.PathError{Op: "open", Path: "x", Err: syscall.ENOENT}
	if got, want := SysErrString(err), "No such file or directory"; got != want {
		t.Errorf("SysErrString(ENOENT) = %q; want %q", got, want)
	}
}
