//go:build windows

package agentpty

import "os/exec"

type parentDeathWatch struct{ cmd *exec.Cmd }

func startParentDeathWatch(_ int) *parentDeathWatch { return nil }
func stopParentDeathWatch(_ *parentDeathWatch)      {}
