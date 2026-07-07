// Package vdircmd implements vdir(1) as an ls -l variant.
package vdircmd

import (
	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/ls"
)

var cmd = &tool.Tool{
	Name:     "vdir",
	Synopsis: "List directory contents in long form.",
	Usage:    "vdir [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	lsArgs := make([]string, 0, len(args)+1)
	lsArgs = append(lsArgs, "-l")
	lsArgs = append(lsArgs, args...)
	return tool.Lookup("ls").Run(rc, lsArgs)
}
