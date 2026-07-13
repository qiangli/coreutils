// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !linux && !darwin && !windows

package steward

import "os"

// lockFile is a no-op on platforms with neither flock nor LockFileEx.
//
// Stated plainly rather than pretended: without mutual exclusion, two processes
// that interleave read-decide-write can both conclude the seat is vacant and both
// claim it. What still holds on such a host is the fencing epoch — the second
// claim wins the journal, the first holder's epoch is superseded, and its next
// mutation is rejected — so a lost acquisition race degrades into a detected
// fencing error rather than two silent stewards. Linux, macOS, and Windows all
// take a real lock (see lock_unix.go / lock_windows.go).
func lockFile(*os.File) (func(), error) {
	return func() {}, nil
}
