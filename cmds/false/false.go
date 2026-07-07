// Package falsecmd implements false(1): do nothing, unsuccessfully.
package falsecmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

func init() {
	cmd := &tool.Tool{
		Name:     "false",
		Synopsis: "Do nothing, unsuccessfully.",
		Usage:    "false [ignored command line arguments]",
	}
	cmd.Run = func(rc *tool.RunContext, args []string) int {
		if len(args) > 0 {
			switch args[0] {
			case "--help", "-h":
				fmt.Fprintf(rc.Out, "Usage: %s\n%s\n\nOptions:\n      --help     display this help and exit\n      --version  output version information and exit\n", cmd.Usage, cmd.Synopsis)
				return 0
			case "--version", "-V":
				fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
				return 0
			}
		}
		return 1
	}
	tool.Register(cmd)
}
