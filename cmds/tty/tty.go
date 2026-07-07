// Package ttycmd implements tty(1) per the GNU coreutils manual:
// print the file name of the terminal connected to standard input, or
// "not a tty" (exit status 1) when standard input is not a terminal.
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
	fs.Bool("quiet", false, "same as --silent")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	if f, ok := rc.In.(*os.File); ok {
		if name, ok := ttyName(f); ok {
			if !*silent && !fs.Changed("quiet") {
				fmt.Fprintf(rc.Out, "%s\n", name)
			}
			return 0
		}
	}
	// GNU prints the diagnosis on stdout, not stderr.
	if !*silent && !fs.Changed("quiet") {
		fmt.Fprintf(rc.Out, "not a tty\n")
	}
	return 1
}
