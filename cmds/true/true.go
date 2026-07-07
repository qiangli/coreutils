// Package truecmd implements true(1): do nothing, successfully.
package truecmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

func init() {
	cmd := &tool.Tool{
		Name:     "true",
		Synopsis: "Do nothing, successfully.",
		Usage:    "true [ignored command line arguments]",
	}
	cmd.Run = func(rc *tool.RunContext, args []string) int {
		if len(args) > 0 {
			switch args[0] {
			case "--help", "-h":
				fmt.Fprintf(rc.Out, "Usage: %s\n%s\n", cmd.Usage, cmd.Synopsis)
			case "--version", "-V":
				fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
			}
		}
		return 0
	}
	tool.Register(cmd)
}
