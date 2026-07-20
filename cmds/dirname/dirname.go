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
	"io"
	"strings"

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
	args = tool.AliasHelpVersion(args)
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
		output := dirOf(name) + end
		n, err := io.WriteString(rc.Out, output)
		if err == nil && n != len(output) {
			err = io.ErrShortWrite
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "dirname: write error: %v\n", err)
			return 1
		}
	}
	return 0
}

// dirOf mirrors GNU dirname exactly: strip trailing slashes, strip
// the last component, strip the trailing slashes that remain. No
// cleaning beyond that — "a/./b" yields "a/." like GNU, which is why
// filepath.Dir (which Cleans) is not used.
func dirOf(name string) string {
	if name == "" {
		return "."
	}

	name = strings.TrimRight(name, "/")
	if name == "" {
		return "/"
	}
	i := strings.LastIndex(name, "/")
	if i < 0 {
		return "."
	}
	result := strings.TrimRight(name[:i], "/")
	if result == "" {
		return "/"
	}
	return result
}
