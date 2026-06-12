// Package dirnamecmd implements dirname(1) per the GNU coreutils
// manual: output each NAME with its last non-slash component and
// trailing slashes removed; if NAME contains no '/', output '.'.
//
// dirname is pure string manipulation (no filesystem access), so the
// behavior is byte-identical on every platform — only '/' is a
// separator, exactly as GNU documents.
package dirnamecmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "dirname",
	Synopsis: "Output each NAME with its last non-slash component and trailing slashes removed.",
	Usage:    "dirname [OPTION] NAME...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	end := "\n"
	if *zero {
		end = "\x00"
	}
	for _, name := range operands {
		fmt.Fprint(rc.Out, dirOf(name), end)
	}
	return 0
}

// dirOf mirrors GNU dirname exactly: strip trailing slashes, strip
// the last component, strip the trailing slashes that remain. No
// cleaning beyond that — "a/./b" yields "a/." like GNU, which is why
// filepath.Dir (which Cleans) is not used.
func dirOf(name string) string {
	i := len(name)
	for i > 0 && name[i-1] == '/' {
		i--
	}
	for i > 0 && name[i-1] != '/' {
		i--
	}
	for i > 0 && name[i-1] == '/' {
		i--
	}
	if i == 0 {
		if len(name) > 0 && name[0] == '/' {
			return "/"
		}
		return "."
	}
	return name[:i]
}
