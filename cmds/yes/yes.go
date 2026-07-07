// Package yescmd implements yes(1) per the GNU coreutils manual:
// repeatedly output a line with all specified STRING(s), or 'y'.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/yes/yes.go (BSD-3-Clause).
// Changes: rewired to the tool framework; honors RunContext.Ctx
// cancellation; exits quietly on write errors (EPIPE); buffers
// repeated lines per write like GNU yes.
package yescmd

import (
	"bytes"
	"context"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "yes",
	Synopsis: "Repeatedly output a line with all specified STRING(s), or 'y'.",
	Usage:    "yes [STRING]...\n   or: yes OPTION",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	line := "y"
	if len(operands) > 0 {
		line = strings.Join(operands, " ")
	}
	out := []byte(line + "\n")
	// Batch several copies per write (GNU yes fills a buffer too); the
	// ctx check between writes is what makes cancellation effective.
	if n := 8192 / len(out); n > 1 {
		out = bytes.Repeat(out, n)
	}

	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			// Cancelled by the embedder: exit quietly.
			return 1
		default:
		}
		if _, err := rc.Out.Write(out); err != nil {
			// Closed pipe (EPIPE) or any write error: exit quietly,
			// matching GNU yes dying silently on SIGPIPE.
			return 1
		}
	}
}
