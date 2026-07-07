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
	"net"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "hostname",
	Synopsis: "Print the system's host name.",
	Usage:    "hostname [OPTION]... [HOSTNAME]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	domain := fs.BoolP("domain", "d", false, "print DNS domain name")
	fqdn := fs.BoolP("fqdn", "f", false, "print fully qualified domain name")
	ip := fs.BoolP("ip-address", "i", false, "print network addresses for the host name")
	short := fs.BoolP("short", "s", false, "print short host name")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
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
	if *short {
		if i := strings.IndexByte(host, '.'); i >= 0 {
			host = host[:i]
		}
		fmt.Fprintf(rc.Out, "%s\n", host)
		return 0
	}
	if *domain {
		if i := strings.IndexByte(host, '.'); i >= 0 && i+1 < len(host) {
			fmt.Fprintf(rc.Out, "%s\n", host[i+1:])
		} else {
			fmt.Fprintln(rc.Out)
		}
		return 0
	}
	if *ip {
		addrs, err := net.LookupHost(host)
		if err != nil {
			fmt.Fprintf(rc.Err, "hostname: cannot resolve host name: %v\n", err)
			return 1
		}
		fmt.Fprintf(rc.Out, "%s\n", strings.Join(addrs, " "))
		return 0
	}
	if *fqdn {
		// Go's portable hostname API already returns the configured host
		// name. DNS canonicalization is platform/network dependent, so keep
		// the local value, matching uutils' fallback behavior.
	}
	fmt.Fprintf(rc.Out, "%s\n", host)
	return 0
}
