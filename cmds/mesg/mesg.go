// Package mesgcmd implements mesg(1): control whether other users may write to
// your terminal.
//
// The state IS the terminal's group-write bit, not a stored preference, so the
// implementation is a stat and a chmod on the controlling terminal. That also
// means the exit status carries the answer: POSIX specifies 0 when messages are
// allowed and 1 when they are not, so `mesg` is usable in a conditional and a
// caller must not read 1 as failure.
package mesgcmd

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mesg",
	Synopsis: "Allow or deny messages written to the terminal.",
	Usage:    "mesg [y|n]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// Seams so the behaviour is testable without a controlling terminal.
var (
	ttyName = defaultTTYName
	statFn  = os.Stat
	chmodFn = os.Chmod
)

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}

	tty, err := ttyName(rc)
	if err != nil {
		fmt.Fprintf(rc.Err, "mesg: %v\n", err)
		return 2
	}
	fi, err := statFn(tty)
	if err != nil {
		fmt.Fprintf(rc.Err, "mesg: %v\n", err)
		return 2
	}
	mode := fi.Mode().Perm()
	allowed := mode&0o020 != 0

	if len(operands) == 0 {
		if allowed {
			if _, err := fmt.Fprintln(rc.Out, "is y"); err != nil {
				fmt.Fprintf(rc.Err, "mesg: %v\n", err)
				return 2
			}
			return 0
		}
		if _, err := fmt.Fprintln(rc.Out, "is n"); err != nil {
			fmt.Fprintf(rc.Err, "mesg: %v\n", err)
			return 2
		}
		return 1
	}

	switch operands[0] {
	case "y":
		if err := chmodFn(tty, mode|0o020); err != nil {
			fmt.Fprintf(rc.Err, "mesg: %v\n", err)
			return 2
		}
		return 0
	case "n":
		if err := chmodFn(tty, mode&^0o020); err != nil {
			fmt.Fprintf(rc.Err, "mesg: %v\n", err)
			return 2
		}
		// The status reports the state AFTER the change: messages are now
		// denied, which is exit 1 by the same rule as the query form.
		return 1
	default:
		return tool.UsageError(rc, cmd, "operand must be y or n, got %q", operands[0])
	}
}

var _ fs.FileMode
