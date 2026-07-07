// Package dircmd implements dir(1) as an ls variant.
package dircmd

import (
	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/ls"
)

var cmd = &tool.Tool{
	Name:     "dir",
	Synopsis: "List directory contents in compact form.",
	Usage:    "dir [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return tool.Lookup("ls").Run(rc, args)
}
