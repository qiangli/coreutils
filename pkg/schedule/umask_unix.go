//go:build !windows

package schedule

import (
	"fmt"
	"os/exec"
)

// commandWithUmask applies a saved creation mask in a short-lived wrapper
// process. Go has no child-only umask field on exec.Cmd; changing the daemon's
// process-global mask would race unrelated jobs and file writes.
func commandWithUmask(command []string, mask uint32, set bool) *exec.Cmd {
	if !set {
		return exec.Command(command[0], command[1:]...)
	}
	args := []string{"-c", `umask "$1"; shift; exec "$@"`, "bashy-schedule", fmt.Sprintf("%04o", mask&0o777)}
	args = append(args, command...)
	// Use an absolute wrapper shell. Resolving "sh" here would consult the
	// scheduler process PATH before the job's deliberately isolated Env applies.
	return exec.Command("/bin/sh", args...)
}
