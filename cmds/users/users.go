package userscmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/cmds/internal/session"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{Name: "users", Synopsis: "Print users currently logged in.", Usage: "users [OPTION]... [FILE]"}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[1])
	}
	path := ""
	if len(operands) == 1 {
		path = rc.Path(operands[0])
	}
	users, err := session.Users(path)
	if err != nil {
		fmt.Fprintf(rc.Err, "users: %v\n", err)
		return 1
	}
	if len(users) > 0 {
		fmt.Fprintln(rc.Out, strings.Join(users, " "))
	}
	return 0
}
