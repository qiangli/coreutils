//go:build !unix

package nicecmd

import "os/exec"

func signaledExitCode(*exec.ExitError) (int, bool) {
	return 0, false
}
