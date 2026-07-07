package hostidcmd

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"net"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "hostid",
	Synopsis: "Print the numeric identifier for the current host.",
	Usage:    "hostid [OPTION]...",
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
	fmt.Fprintf(rc.Out, "%08x\n", hostID())
	return 0
}

func hostID() uint32 {
	if ifaces, err := net.Interfaces(); err == nil {
		for _, iface := range ifaces {
			if len(iface.HardwareAddr) >= 4 && iface.Flags&net.FlagLoopback == 0 {
				return binary.BigEndian.Uint32(iface.HardwareAddr[len(iface.HardwareAddr)-4:])
			}
		}
	}
	name, _ := os.Hostname()
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return h.Sum32()
}
