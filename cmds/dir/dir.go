package dircmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/coreutils/cmds/ls"
)

var cmd = &tool.Tool{
	Name:     "dir",
	Synopsis: "List directory contents (accepts the same options as ls).",
	Usage:    "dir [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	rest, short := lscmd.ExtractShort(args, "ltS1gGnoCpfUXQNbqsvCxZHLV")
	fs := lscmd.GetFlagSet(cmd.Name)
	_, code := tool.Parse(rc, cmd, fs, rest)
	if code >= 0 {
		return code
	}
	if short['V'] > 0 {
		fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
		return 0
	}
	return tool.Lookup("ls").Run(rc, args)
}
