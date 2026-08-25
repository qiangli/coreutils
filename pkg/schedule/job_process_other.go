//go:build !unix

package schedule

import "os/exec"

func applyJobProcAttrs(*exec.Cmd) {}
