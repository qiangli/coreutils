// Package unlinkcmd implements unlink(1) per the GNU coreutils manual:
// call the unlink function to remove the specified FILE. No options
// besides --help/--version, exactly one operand.
//
// Portions adapted from https://github.com/guonaihong/coreutils unlink/unlink.go (Apache-2.0).
// Changes: rewired to tool framework; refuses directories explicitly
// (os.Remove would delete an empty directory, unlink(2) never does);
// path resolved against the RunContext.
package unlinkcmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "unlink",
	Synopsis: "Call the unlink function to remove the specified FILE.",
	Usage:    "unlink FILE",
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
		// fall through
	default:
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[1])
	}
	name := operands[0]
	path := rc.Path(name)
	// unlink(2) never removes directories; os.Remove would.
	if fi, err := os.Lstat(path); err == nil && fi.IsDir() {
		fmt.Fprintf(rc.Err, "unlink: cannot unlink '%s': Is a directory\n", name)
		return 1
	}
	if err := os.Remove(path); err != nil {
		fmt.Fprintf(rc.Err, "unlink: cannot unlink '%s': %v\n", name, reason(err))
		return 1
	}
	return 0
}

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	return tool.SysErr(err)
}
