// Package unamecmd implements uname(1) per POSIX.1-2016 (Issue 7):
// print system information. Required flags: -a -m -n -r -s -v.
// -o/-p/-i are GNU extensions carried as explicitly requested options.
//
// On unix the values come from uname(2) (golang.org/x/sys/unix), so
// -s/-r/-m report what the platform's own uname reports (e.g. "Darwin
// ... arm64" on Apple Silicon). On Windows the kernel name is
// "Windows_NT", the release is "major.minor.build" from RtlGetVersion,
// the version is "Build N", and the machine maps GOARCH to the GNU spelling
// (x86_64, aarch64).
//
// -a is exactly -mnrsv, as Issue 7 requires ("Behave as though all of
// the options -mnrsv were specified"), and prints the selected symbols
// in the required s n r v m order. The non-POSIX -o/-p/-i fields are
// printed only when explicitly requested, never implied by -a. The
// version symbol is implementation-defined, as POSIX permits.
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
	sysname          string // -s: kernel name
	nodename         string // -n: network node hostname
	release          string // -r: kernel release
	version          string // -v: implementation-defined kernel/OS version
	machine          string // -m: machine hardware name
	processor        string // -p: processor type
	hardwarePlatform string // -i: hardware platform
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.BoolP("all", "a", false, "behave as though -mnrsv were specified")
	kernelName := fs.BoolP("kernel-name", "s", false, "print the kernel name")
	nodename := fs.BoolP("nodename", "n", false, "print the network node hostname")
	release := fs.BoolP("kernel-release", "r", false, "print the kernel release")
	kernelVersion := fs.BoolP("kernel-version", "v", false, "print the kernel version")
	machine := fs.BoolP("machine", "m", false, "print the machine hardware name")
	processor := fs.BoolP("processor", "p", false, "print the processor type (non-portable)")
	hardwarePlatform := fs.BoolP("hardware-platform", "i", false, "print the hardware platform (non-portable)")
	osFlag := fs.BoolP("operating-system", "o", false, "print the operating system")

	// Obsolescent long aliases
	sysnameAlias := fs.Bool("sysname", false, "print the kernel name")
	releaseAlias := fs.Bool("release", false, "print the kernel release")
	if flag := fs.Lookup("sysname"); flag != nil {
		flag.Hidden = true
	}
	if flag := fs.Lookup("release"); flag != nil {
		flag.Hidden = true
	}

	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	if *sysnameAlias {
		*kernelName = true
	}
	if *releaseAlias {
		*release = true
	}

	if !*all && !*kernelName && !*nodename && !*release && !*kernelVersion && !*machine && !*processor && !*hardwarePlatform && !*osFlag {
		*kernelName = true
	}

	info, err := probe()
	if err != nil {
		fmt.Fprintf(rc.Err, "uname: cannot get system name: %v\n", err)
		return 1
	}

	parts := assemble(info, selection{
		all:              *all,
		sysname:          *kernelName,
		nodename:         *nodename,
		release:          *release,
		version:          *kernelVersion,
		machine:          *machine,
		processor:        *processor,
		hardwarePlatform: *hardwarePlatform,
		operatingSystem:  *osFlag,
	})
	fmt.Fprintf(rc.Out, "%s\n", strings.Join(parts, " "))
	return 0
}

// selection records which output symbols were requested.
type selection struct {
	all              bool
	sysname          bool
	nodename         bool
	release          bool
	version          bool
	machine          bool
	processor        bool
	hardwarePlatform bool
	operatingSystem  bool
}

// assemble builds the output fields in the fixed Issue 7 order
// (sysname nodename release version machine), independent of the order
// the flags were typed. -a behaves as though -mnrsv were specified and
// selects nothing else: -o, -p, and -i are extensions and stay opt-in.
// A failed or synthetic probe can still lack a version value; skip an empty
// field defensively so repeated selectors never introduce an empty column.
func assemble(info sysinfo, sel selection) []string {
	var parts []string
	if sel.sysname || sel.all {
		parts = append(parts, info.sysname)
	}
	if sel.nodename || sel.all {
		parts = append(parts, info.nodename)
	}
	if sel.release || sel.all {
		parts = append(parts, info.release)
	}
	if (sel.version || sel.all) && info.version != "" {
		parts = append(parts, info.version)
	}
	if sel.machine || sel.all {
		parts = append(parts, info.machine)
	}
	if sel.processor {
		parts = append(parts, info.processor)
	}
	if sel.hardwarePlatform {
		parts = append(parts, info.hardwarePlatform)
	}
	if sel.operatingSystem {
		parts = append(parts, operatingSystem())
	}
	return parts
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
