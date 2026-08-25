//go:build !unix

package mvcmd

import "os"

// rename falls back to os.Rename where the platform has no POSIX rename(2):
// on Windows MoveFileEx cannot replace an existing directory at all, so the
// standard library's existing-directory refusal is already the platform
// behavior and the failure is surfaced as a diagnostic.
func rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
