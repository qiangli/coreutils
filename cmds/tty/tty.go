// Package ttycmd implements tty(1) per the GNU coreutils manual:
// print the file name of the terminal connected to standard input, or
// "not a tty" (exit status 1) when standard input is not a terminal.
// Exit statuses follow the GNU manual: 0 terminal, 1 non-terminal,
// 2 incorrect arguments, 3 write error.
//
// The check runs against RunContext.In: it must be an *os.File whose
// descriptor answers a terminal ioctl. On Windows a console handle
// counts as a terminal and prints "CON" (the console device name —
// Windows has no per-terminal path for GNU tty to report).
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/tty/tty.go (BSD-3-Clause).
// Changes: rewired to the tool framework over RunContext.In;
// /proc/self/fd readlink on Linux instead of a /dev walk; rdev-match
// /dev scan on darwin; Windows console-handle path added.
package ttycmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tty",
	Synopsis: "Print the file name of the terminal connected to standard input.",
	Usage:    "tty [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	silent := fs.BoolP("silent", "s", false, "print nothing, only return an exit status")
	quiet := fs.Bool("quiet", false, "same as --silent")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	name, isTTY := "", false
	if f, ok := rc.In.(*os.File); ok {
		name, isTTY = ttyName(f)
	}
	if !*silent && !*quiet {
		// GNU prints the diagnosis on stdout, not stderr; a write
		// error there is exit status 3 per the GNU manual, taking
		// precedence over the terminal/non-terminal status.
		line := name
		if !isTTY {
			line = "not a tty"
		}
		if _, err := fmt.Fprintf(rc.Out, "%s\n", line); err != nil {
			fmt.Fprintf(rc.Err, "tty: write error: %v\n", err)
			return 3
		}
	}
	if isTTY {
		return 0
	}
	return 1
}
