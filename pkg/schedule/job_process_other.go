//go:build !unix

package schedule

import "os/exec"

func applyJobProcAttrs(*exec.Cmd) {}

func budgetOwnedJobGone(cmd *exec.Cmd) bool { return cmd == nil || cmd.Process == nil }
