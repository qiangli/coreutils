// Package vdircmd implements vdir(1) as an ls -l variant.
//
// Documented deviation: GNU vdir is `ls -l -b`; this ls has no
// backslash-escape mode, so vdir delegates to ls -l.
package vdircmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/ls"
)

var cmd = &tool.Tool{
	Name:     "vdir",
	Synopsis: "List directory contents in long form (accepts the same options as ls).",
	Usage:    "vdir [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if code := answerIdentity(rc, cmd, args); code >= 0 {
		return code
	}
	lsArgs := make([]string, 0, len(args)+1)
	lsArgs = append(lsArgs, "-l")
	lsArgs = append(lsArgs, args...)
	return tool.Lookup("ls").Run(rc, lsArgs)
}

// answerIdentity handles --help/--version under the delegating tool's
// own name (delegating them to ls would misreport the identity).
// Returns -1 to proceed with delegation.
func answerIdentity(rc *tool.RunContext, t *tool.Tool, args []string) int {
	for _, a := range args {
		if a == "--" {
			break
		}
		switch a {
		case "--help":
			fmt.Fprintf(rc.Out, "Usage: %s\n", t.Usage)
			fmt.Fprintf(rc.Out, "%s\n", t.Synopsis)
			fmt.Fprintf(rc.Out, "\nOptions are those of ls — see 'ls --help' for the supported subset.\n")
			return 0
		case "--version":
			fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", t.Name, tool.Version)
			return 0
		}
	}
	return -1
}
