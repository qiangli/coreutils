// Package falsecmd implements false(1): do nothing, unsuccessfully.
package falsecmd

import "github.com/qiangli/coreutils/tool"

func init() {
	tool.Register(&tool.Tool{
		Name:     "false",
		Synopsis: "Do nothing, unsuccessfully.",
		Usage:    "false [ignored command line arguments]",
		Run:      func(rc *tool.RunContext, args []string) int { return 1 },
	})
}
