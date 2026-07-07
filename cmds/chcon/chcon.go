// Package chconcmd implements a narrow chcon(1) subset: set the
// SELinux security context of each FILE by writing security.selinux.
package chconcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chcon",
	Synopsis: "Change the SELinux security context of each FILE.",
	Usage:    "chcon CONTEXT FILE...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	if len(operands) == 1 {
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	}
	return applyContext(rc, operands[0], operands[1:])
}

func reportChconErr(rc *tool.RunContext, display, context string, err error) {
	fmt.Fprintf(rc.Err, "chcon: failed to change context of '%s' to '%s': %v\n", display, context, tool.SysErr(err))
}
