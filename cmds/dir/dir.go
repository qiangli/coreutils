package dircmd

import (
	"fmt"

	"github.com/qiangli/coreutils/cmds/ls"
	"github.com/qiangli/coreutils/tool"
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
	// GNU dir is an ls alias with these defaults placed before all explicit
	// arguments, so later user options retain their normal precedence.
	lsArgs := make([]string, 0, len(args)+2)
	lsArgs = append(lsArgs, "-C", "-b")
	lsArgs = append(lsArgs, args...)
	return tool.Lookup("ls").Run(rc, lsArgs)
}
