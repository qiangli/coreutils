// Package mkfifocmd implements mkfifo(1): create named pipes.
//
// This is a conservative subset: -m/--mode accepts octal modes only.
// The native operation is split behind build tags so non-Unix platforms
// fail loudly instead of approximating FIFO semantics.
// -Z/--context accepted as no-op on non-SELinux platforms.
package mkfifocmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mkfifo",
	Synopsis: "Create named pipes (FIFOs).",
	Usage:    "mkfifo [OPTION]... NAME...",
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
	if len(operands) == 0 {
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

	status := 0
	for _, name := range operands {
		if err := makeFIFO(rc.Path(name), mode); err != nil {
			fmt.Fprintf(rc.Err, "mkfifo: cannot create fifo '%s': %s\n", name, tool.SysErrString(err))
			status = 1
			continue
		}
		if useMode {
			if err := os.Chmod(rc.Path(name), fileMode(mode)); err != nil {
				fmt.Fprintf(rc.Err, "mkfifo: cannot set permissions of '%s': %s\n", name, tool.SysErrString(err))
				status = 1
			}
		}
	}
	return status
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
