// Package unamecmd implements uname(1) per the GNU coreutils manual:
// print system information. Supported flags: -a -s -n -r -m -o.
//
// On unix the values come from uname(2) (golang.org/x/sys/unix), so
// -s/-r/-m report what the platform's own uname reports (e.g. "Darwin
// ... arm64" on Apple Silicon). On Windows the kernel name is
// "Windows_NT", the release is "major.minor.build" from RtlGetVersion,
// and the machine maps GOARCH to the GNU spelling (x86_64, aarch64).
//
// -a prints kernel name, nodename, release, kernel version, machine,
// and operating system; the kernel-version field is omitted on
// platforms that do not provide one (Windows), the same way GNU -a
// omits unknown -p/-i fields.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/uname/uname.go (BSD-3-Clause)
// and https://github.com/guonaihong/coreutils uname/uname.go (Apache-2.0).
// Changes: rewired to the tool framework; Windows probe added; -o
// operating-system names per GNU spellings.
package unamecmd

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "uname",
	Synopsis: "Print certain system information (default: the kernel name).",
	Usage:    "uname [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// sysinfo is filled by the per-platform probe.
type sysinfo struct {
	sysname  string // -s: kernel name
	nodename string // -n: network node hostname
	release  string // -r: kernel release
	version  string // kernel version; printed only by -a, "" = omit
	machine  string // -m: machine hardware name
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "print all information, in the following order, except omit unknown fields")
	kernelName := fs.BoolP("kernel-name", "s", false, "print the kernel name")
	nodename := fs.BoolP("nodename", "n", false, "print the network node hostname")
	release := fs.BoolP("kernel-release", "r", false, "print the kernel release")
	kernelVersion := fs.BoolP("kernel-version", "v", false, "print the kernel version")
	machine := fs.BoolP("machine", "m", false, "print the machine hardware name")
	osFlag := fs.BoolP("operating-system", "o", false, "print the operating system")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	if !*all && !*kernelName && !*nodename && !*release && !*kernelVersion && !*machine && !*osFlag {
		*kernelName = true
	}

	info, err := probe()
	if err != nil {
		fmt.Fprintf(rc.Err, "uname: cannot get system name: %v\n", err)
		return 1
	}

	var parts []string
	if *kernelName || *all {
		parts = append(parts, info.sysname)
	}
	if *nodename || *all {
		parts = append(parts, info.nodename)
	}
	if *release || *all {
		parts = append(parts, info.release)
	}
	if *all && info.version != "" {
		parts = append(parts, info.version)
	}
	if *kernelVersion {
		parts = append(parts, info.version)
	}
	if *machine || *all {
		parts = append(parts, info.machine)
	}
	if *osFlag || *all {
		parts = append(parts, operatingSystem())
	}
	fmt.Fprintf(rc.Out, "%s\n", strings.Join(parts, " "))
	return 0
}

// operatingSystem maps GOOS to the GNU -o spelling.
func operatingSystem() string {
	switch runtime.GOOS {
	case "linux":
		return "GNU/Linux"
	case "darwin":
		return "Darwin"
	case "windows":
		return "Windows_NT"
	case "freebsd":
		return "FreeBSD"
	case "openbsd":
		return "OpenBSD"
	case "netbsd":
		return "NetBSD"
	case "android":
		return "Android"
	default:
		return runtime.GOOS
	}
}

// gnuArch maps GOARCH to the GNU machine spelling, for platforms
// where uname(2) is unavailable.
func gnuArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7l"
	default:
		return runtime.GOARCH
	}
}
