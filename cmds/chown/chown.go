// Package chowncmd implements chown(1) per the GNU coreutils manual:
// change file owner and group ([OWNER][:[GROUP]] by name or numeric
// ID), with -R.
//
// Unix only: Windows has no uid/gid ownership model, so the Windows
// build fails loudly instead (see chown_windows.go).
//
// Portions adapted from https://github.com/guonaihong/coreutils chown/chown.go (Apache-2.0).
// Changes: rewired to tool framework; OWNER[:GROUP] parsing rewritten
// around strings.Cut with name-then-numeric lookup via os/user;
// '.' separator and verbose/from/reference flags dropped; recursion
// uses filepath.WalkDir with lchown for traversed symlinks.
package chowncmd

import (
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "chown",
	Synopsis: "Change the owner and/or group of each FILE to OWNER and/or GROUP.",
	Usage:    "chown [OPTION]... [OWNER][:[GROUP]] FILE...",
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
