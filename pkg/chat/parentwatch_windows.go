//go:build windows

package chat

import "os/exec"

// Windows has no portable parent-death notification for this lightweight
// runner. The process-group/job cleanup remains the platform-specific fallback.
type parentDeathWatch struct{ cmd *exec.Cmd }

func startParentDeathWatch(_ int) *parentDeathWatch { return nil }
func stopParentDeathWatch(_ *parentDeathWatch)      {}
