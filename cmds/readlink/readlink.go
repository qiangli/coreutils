// Package readlinkcmd implements readlink(1) per the GNU coreutils
// manual: print the value of each symbolic link. With no mode flag
// the operand must itself be a symlink and its stored target is
// printed exactly; -f/-e/-m canonicalize instead (recursive symlink
// resolution, existence requirement varying by flag).
//
// GNU suppresses error messages by default (-q is the default; -v is
// not implemented), so failures are silent and only the exit status
// reports them.
package readlinkcmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "readlink",
	Synopsis: "Print value of a symbolic link or canonical file name.",
	Usage:    "readlink [OPTION]... FILE...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	fs.BoolP("canonicalize", "f", false, "canonicalize: all but the last component must exist")
	fs.BoolP("canonicalize-existing", "e", false, "canonicalize: all components must exist")
	fs.BoolP("canonicalize-missing", "m", false, "canonicalize: no components need exist")
	noNewline := fs.BoolP("no-newline", "n", false, "do not output the trailing delimiter")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	nn := *noNewline
	if nn && len(operands) > 1 {
		// GNU warns and re-enables the delimiter.
		fmt.Fprintln(rc.Err, "readlink: ignoring --no-newline with multiple arguments")
		nn = false
	}

	mode := lastFEM(args) // GNU: the last of -f/-e/-m wins; 0 = plain readlink
	status := 0
	for _, op := range operands {
		out, err := resolveOne(rc, op, mode)
		if err != nil {
			status = 1 // quiet by default, like GNU
			continue
		}
		if nn {
			fmt.Fprint(rc.Out, out)
		} else {
			fmt.Fprintln(rc.Out, out)
		}
	}
	return status
}

func resolveOne(rc *tool.RunContext, operand string, mode byte) (string, error) {
	if mode == 0 {
		// Plain mode: the operand must be a symlink; print its stored
		// target verbatim (relative targets stay relative).
		return os.Readlink(rc.Path(operand))
	}
	abs, err := absOperand(rc, operand)
	if err != nil {
		return "", err
	}
	switch mode {
	case 'e':
		return canonicalize(abs, canonExisting)
	case 'm':
		return canonicalize(abs, canonMissing)
	default: // 'f'
		return canonicalize(abs, canonAllButLast)
	}
}

// lastFEM scans args for the last occurrence of -f/-e/-m (or long
// forms) — pflag reports values but not order, and GNU readlink lets
// the last mode flag win.
func lastFEM(args []string) byte {
	var mode byte
	for _, a := range args {
		switch {
		case a == "--":
			return mode
		case a == "--canonicalize":
			mode = 'f'
		case a == "--canonicalize-existing":
			mode = 'e'
		case a == "--canonicalize-missing":
			mode = 'm'
		case len(a) > 1 && a[0] == '-' && a[1] != '-':
			for _, c := range a[1:] {
				switch c {
				case 'f', 'e', 'm':
					mode = byte(c)
				}
			}
		}
	}
	return mode
}
