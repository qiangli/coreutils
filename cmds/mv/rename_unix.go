//go:build unix

package mvcmd

import (
	"os"
	"syscall"
)

// rename performs the rename(2) equivalence POSIX mv requires. It cannot use
// os.Rename: the Go standard library pre-checks the destination and refuses
// with EEXIST whenever it is an existing directory, without ever calling
// rename(2) — but POSIX rename() (and therefore mv) must atomically replace
// an existing empty destination directory with a source directory. Calling
// the syscall directly restores the kernel's semantics: empty destination
// directories are replaced, non-empty ones fail with ENOTEMPTY/EEXIST, and a
// file-over-directory rename fails with EISDIR, each surfaced as a
// diagnostic.
func rename(oldpath, newpath string) error {
	if err := syscall.Rename(oldpath, newpath); err != nil {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}
	return nil
}
