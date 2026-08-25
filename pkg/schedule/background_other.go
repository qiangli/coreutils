//go:build !unix

package schedule

import (
	"os/exec"
)

// applyBackgroundProcAttrs is a no-op on non-unix (no process groups).
func applyBackgroundProcAttrs(cmd *exec.Cmd) {}
