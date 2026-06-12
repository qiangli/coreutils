// Package hostnamecmd implements hostname(1) (print mode only): show
// the system's host name. Setting the host name (the HOSTNAME
// operand) is documented-but-unsupported — it requires privileged
// platform calls outside this toolset's scope.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/hostname/hostname.go (BSD-3-Clause).
// Changes: rewired to the tool framework; set mode refused with the
// contract error.
package hostnamecmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "hostname",
	Synopsis: "Print the system's host name.",
	Usage:    "hostname",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.NotSupported(rc, cmd, "setting the host name")
	}

	host, err := os.Hostname()
	if err != nil {
		fmt.Fprintf(rc.Err, "hostname: cannot determine host name: %v\n", err)
		return 1
	}
	fmt.Fprintf(rc.Out, "%s\n", host)
	return 0
}
