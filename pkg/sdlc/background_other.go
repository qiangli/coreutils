//go:build !unix

package sdlc

import "os/exec"

func applyBackgroundProcAttrs(cmd *exec.Cmd) {}
