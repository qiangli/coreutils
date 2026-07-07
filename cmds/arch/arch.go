package archcmd

import (
	"fmt"
	"runtime"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "arch",
	Synopsis: "Print machine hardware name.",
	Usage:    "arch [OPTION]...",
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
	fmt.Fprintln(rc.Out, machine())
	return 0
}

func machine() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7l"
	case "ppc64le":
		return "ppc64le"
	case "s390x":
		return "s390x"
	default:
		return runtime.GOARCH
	}
}
