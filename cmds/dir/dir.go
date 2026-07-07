// Package dircmd implements dir(1) as an ls variant.
//
// Documented deviation: GNU dir is `ls -C -b`; this ls has no column
// or backslash-escape modes (deterministic one-per-line output), so
// dir delegates to plain ls.
package dircmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/ls"
)

var cmd = &tool.Tool{
	Name:     "dir",
	Synopsis: "List directory contents (accepts the same options as ls).",
	Usage:    "dir [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	if code := answerIdentity(rc, cmd, args); code >= 0 {
		return code
	}
	return tool.Lookup("ls").Run(rc, args)
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
