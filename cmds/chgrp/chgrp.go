// Package chgrpcmd implements chgrp(1) per the GNU coreutils manual:
// change the group of each FILE to GROUP (name or numeric id), with -R.
//
// Unix only: Windows has no gid ownership model, so the Windows build
// fails loudly instead (see chgrp_windows.go).
//
// Portions adapted from https://github.com/guonaihong/coreutils chgrp/chgrp.go (Apache-2.0).
// Changes: rewired to tool framework; group lookup is name-then-numeric
// via os/user; verbose/reference flags dropped; recursion uses
// filepath.WalkDir with lchown for traversed symlinks.
package chgrpcmd

import (
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chgrp",
	Synopsis: "Change the group of each FILE to GROUP.",
	Usage:    "chgrp [OPTION]... GROUP FILE...",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	recursive := fs.BoolP("recursive", "R", false, "operate on files and directories recursively")
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
	return apply(rc, operands[0], operands[1:], *recursive)
}
