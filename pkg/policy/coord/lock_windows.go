// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package coord

import (
	"fmt"
	"os"
)

// withLock on Windows is best-effort: there is no flock, and weave's queue lock has
// the same gap.
//
// Say so plainly rather than pretending: without mutual exclusion, two agents that
// interleave read-decide-write can BOTH conclude the project is free. The claim
// still works — a conflicting claim written second is still visible to the next
// reader, and the refusal still fires — but the acquisition itself is racy under
// genuine simultaneity.
//
// Closing it needs LockFileEx (golang.org/x/sys/windows), which is tracked. Until
// then a Windows host is protected by everything EXCEPT the last microsecond.
func withLock(dir string, fn func() (*Claim, error)) (*Claim, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("claim lock: %w", err)
	}
	return fn()
}
