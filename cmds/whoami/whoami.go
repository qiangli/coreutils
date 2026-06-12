// Package whoamicmd implements whoami(1) per the GNU coreutils
// manual: print the user name associated with the current effective
// user ID.
//
// The name comes from os/user (the OS account database), never from
// $USER. On Windows os/user reports "DOMAIN\name"; the domain
// qualifier is stripped so the output matches what GNU whoami prints
// in Windows environments (the bare account name).
package whoamicmd

import (
	"fmt"
	"os/user"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "whoami",
	Synopsis: "Print the user name associated with the current effective user ID.",
	Usage:    "whoami [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	u, err := user.Current()
	if err != nil {
		fmt.Fprintf(rc.Err, "whoami: cannot find name for the current user: %v\n", err)
		return 1
	}
	name := u.Username
	if runtime.GOOS == "windows" {
		if i := strings.LastIndexByte(name, '\\'); i >= 0 {
			name = name[i+1:]
		}
	}
	fmt.Fprintf(rc.Out, "%s\n", name)
	return 0
}
