// Package mknodcmd implements mknod(1): create special files.
//
// This is a conservative GNU-compatible slice: NAME TYPE [MAJOR MINOR],
// with TYPE p for FIFOs and b/c/u for block or character devices.
// -m/--mode accepts octal modes only.
// -Z/--context accepted as no-op on non-SELinux platforms.
package mknodcmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mknod",
	Synopsis: "Create the special file NAME of the given TYPE.",
	Usage:    "mknod [OPTION]... NAME TYPE [MAJOR MINOR]",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	modeStr := fs.StringP("mode", "m", "", "set file mode (octal, as in chmod), not a=rw - umask")
	contextStr := fs.StringP("context", "Z", "", "set SELinux security context (no-op without SELinux)")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	_ = contextStr // deterministic no-op on non-SELinux platforms
	if len(operands) < 2 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	mode := uint32(0o666)
	useMode := fs.Changed("mode")
	if useMode {
		var errCode int
		mode, errCode = parseOctalMode(rc, *modeStr)
		if errCode >= 0 {
			return errCode
		}
	}

	spec, code := parseNodeSpec(rc, operands, mode)
	if code >= 0 {
		return code
	}
	if err := makeNode(rc.Path(spec.name), spec.kind, spec.mode, spec.major, spec.minor); err != nil {
		fmt.Fprintf(rc.Err, "mknod: %s '%s': %s\n", spec.errVerb, spec.name, tool.SysErrString(err))
		return 1
	}
	if useMode {
		if err := os.Chmod(rc.Path(spec.name), fileMode(mode)); err != nil {
			fmt.Fprintf(rc.Err, "mknod: cannot set permissions of '%s': %s\n", spec.name, tool.SysErrString(err))
			return 1
		}
	}
	return 0
}

type nodeSpec struct {
	name    string
	kind    nodeKind
	mode    uint32
	major   uint32
	minor   uint32
	errVerb string
}

type nodeKind int

const (
	nodeFIFO nodeKind = iota
	nodeBlock
	nodeChar
)

func parseNodeSpec(rc *tool.RunContext, operands []string, mode uint32) (nodeSpec, int) {
	spec := nodeSpec{name: operands[0], mode: mode}
	switch operands[1] {
	case "p":
		if len(operands) > 2 {
			return spec, tool.UsageError(rc, cmd, "extra operand '%s'", operands[2])
		}
		spec.kind = nodeFIFO
		spec.errVerb = "cannot create fifo"
		return spec, -1
	case "b":
		spec.kind = nodeBlock
		spec.errVerb = "cannot create block special file"
	case "c", "u":
		spec.kind = nodeChar
		spec.errVerb = "cannot create character special file"
	default:
		return spec, tool.UsageError(rc, cmd, "invalid device type '%s'", operands[1])
	}

	if len(operands) < 4 {
		return spec, tool.UsageError(rc, cmd, "missing operand after '%s'", operands[len(operands)-1])
	}
	if len(operands) > 4 {
		return spec, tool.UsageError(rc, cmd, "extra operand '%s'", operands[4])
	}
	major, errCode := parseDeviceNumber(rc, "major", operands[2])
	if errCode >= 0 {
		return spec, errCode
	}
	minor, errCode := parseDeviceNumber(rc, "minor", operands[3])
	if errCode >= 0 {
		return spec, errCode
	}
	spec.major = major
	spec.minor = minor
	return spec, -1
}

func parseDeviceNumber(rc *tool.RunContext, label, s string) (uint32, int) {
	n, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return 0, tool.UsageError(rc, cmd, "invalid %s device number '%s'", label, s)
	}
	return uint32(n), -1
}

func parseOctalMode(rc *tool.RunContext, s string) (uint32, int) {
	n, err := strconv.ParseUint(s, 8, 32)
	if err == nil && n <= 0o7777 {
		return uint32(n), -1
	}
	if s == "" || allDigits(s) {
		return 0, tool.UsageError(rc, cmd, "invalid mode '%s'", s)
	}
	return 0, tool.NotSupported(rc, cmd, fmt.Sprintf("symbolic mode '%s' for -m/--mode (only octal modes)", s))
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func fileMode(n uint32) os.FileMode {
	mode := os.FileMode(n & 0o777)
	if n&0o1000 != 0 {
		mode |= os.ModeSticky
	}
	if n&0o2000 != 0 {
		mode |= os.ModeSetgid
	}
	if n&0o4000 != 0 {
		mode |= os.ModeSetuid
	}
	return mode
}
