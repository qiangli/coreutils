package tool

import (
	"os"
	"syscall"

	"mvdan.cc/sh/v3/execbudget"
)

// CheckExecBudget applies Bashy's advertised combined argv and environment
// budget to applets that launch a child directly. The host can still reject a
// launch for a stricter native limit.
func CheckExecBudget(argv, env []string) error {
	// Like exec.Cmd, a nil Env inherits the current process environment;
	// a nonnil empty slice deliberately passes an empty environment.
	if env == nil {
		env = os.Environ()
	}
	if execbudget.OverBashyArgMax(argv, env) {
		return syscall.E2BIG
	}
	return nil
}
