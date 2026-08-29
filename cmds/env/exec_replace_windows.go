//go:build windows

package envcmd

import (
	"os"

	"github.com/qiangli/coreutils/tool"
)

// Windows has no execve process overlay. Dedicated and embedded invocations
// both use the portable fork/wait implementation.
func replaceCommand(*tool.RunContext, string, []string, []string, string, []os.Signal) (bool, error) {
	return false, nil
}
