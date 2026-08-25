//go:build !unix

package nicecmd

import "os/exec"

func signaledExitCode(*exec.ExitError) (int, int, bool) {
	return 0, 0, false
}
