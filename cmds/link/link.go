// Package linkcmd implements link(1) per the GNU coreutils manual:
// call the link function to create a link named FILE2 to an existing
// FILE1. No options besides --help/--version, exactly two operands.
//
// Portions adapted from https://github.com/guonaihong/coreutils link/link.go (Apache-2.0).
// Changes: rewired to tool framework; operand arity errors use the
// usage-error contract; paths resolved against the RunContext.
package linkcmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "link",
	Synopsis: "Call the link function to create a link named FILE2 to an existing FILE1.",
	Usage:    "link FILE1 FILE2",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	switch len(operands) {
	case 0:
		return tool.UsageError(rc, cmd, "missing operand")
	case 1:
		return tool.UsageError(rc, cmd, "missing operand after '%s'", operands[0])
	case 2:
		// fall through
	default:
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
	}
	if err := os.Link(rc.Path(operands[0]), rc.Path(operands[1])); err != nil {
		fmt.Fprintf(rc.Err, "link: cannot create link '%s' to '%s': %v\n", operands[1], operands[0], reason(err))
		return 1
	}
	return 0
}

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	return tool.SysErr(err)
}
