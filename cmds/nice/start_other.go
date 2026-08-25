//go:build !unix

package nicecmd

import (
	"fmt"
	"os/exec"

	"github.com/qiangli/coreutils/tool"
)

// Platforms without POSIX process priorities retain the truthful boundary:
// invoke the utility unchanged and emit the permitted warning.
func startPriorityCommand(rc *tool.RunContext, name, path string, args []string, niceness int) (*exec.Cmd, error) {
	c, err := rc.StartCommand(path, args, rc.In, rc.Out, rc.Err)
	if err == nil {
		if priorityErr := setPriority(c.Process.Pid, niceness); priorityErr != nil {
			fmt.Fprintf(rc.Err, "%s: cannot set niceness: %v\n", name, priorityErr)
		}
	}
	return c, err
}
