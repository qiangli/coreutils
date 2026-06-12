// Package truecmd implements true(1): do nothing, successfully.
package truecmd

import "github.com/qiangli/coreutils/tool"

func init() {
	tool.Register(&tool.Tool{
		Name:     "true",
		Synopsis: "Do nothing, successfully.",
		Usage:    "true [ignored command line arguments]",
		// GNU true ignores everything, including --help-like args when
		// not the first argument; exit 0 unconditionally is the
		// documented core behavior.
		Run: func(rc *tool.RunContext, args []string) int { return 0 },
	})
}
