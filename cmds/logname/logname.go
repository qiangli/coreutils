package lognamecmd

import (
	"fmt"
	"os"
	"os/user"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "logname",
	Synopsis: "Print the user's login name.",
	Usage:    "logname [OPTION]...",
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
	name := loginName()
	if name == "" {
		fmt.Fprintln(rc.Err, "logname: no login name")
		return 1
	}
	fmt.Fprintln(rc.Out, name)
	return 0
}

func loginName() string {
	for _, key := range []string{"LOGNAME", "USER", "LNAME", "USERNAME"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return bareUser(v)
		}
	}
	if u, err := user.Current(); err == nil {
		return bareUser(u.Username)
	}
	return ""
}

func bareUser(s string) string {
	if runtime.GOOS == "windows" {
		if i := strings.LastIndexByte(s, '\\'); i >= 0 {
			return s[i+1:]
		}
	}
	return s
}
