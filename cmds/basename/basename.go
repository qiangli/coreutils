// Package basenamecmd implements basename(1) per the GNU coreutils
// manual: strip directory and (optionally) suffix from each NAME.
//
// Exemplar for the cmds/ pattern: package <name>cmd in cmds/<name>/,
// init-time registration, tool.NewFlags + tool.Parse, every operand
// resolved logically (basename is pure string manipulation — tools
// that touch the filesystem must use rc.Path).
package basenamecmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "basename",
	Synopsis: "Print NAME with any leading directory components removed.",
	Usage:    "basename NAME [SUFFIX]\n   or: basename OPTION... NAME...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	args = tool.AliasHelpVersion(args)
	fs := tool.NewFlags(cmd.Name)
	multiple := fs.BoolP("multiple", "a", false, "support multiple arguments and treat each as a NAME")
	suffix := fs.StringP("suffix", "s", "", "remove a trailing SUFFIX; implies -a")
	zero := fs.BoolP("zero", "z", false, "end each output line with NUL, not newline")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	suf := *suffix
	names := operands
	// Two-operand classic form: basename NAME SUFFIX — only when
	// neither -a nor -s was given (GNU semantics). An explicitly empty
	// suffix still implies -a.
	if !*multiple && !fs.Changed("suffix") {
		if len(operands) > 2 {
			return tool.UsageError(rc, cmd, "extra operand %q", operands[2])
		}
		names = operands[:1]
		if len(operands) == 2 {
			suf = operands[1]
		}
	}

	end := "\n"
	if *zero {
		end = "\x00"
	}
	for _, name := range names {
		output := base(name, suf) + end
		n, err := io.WriteString(rc.Out, output)
		if err == nil && n != len(output) {
			err = io.ErrShortWrite
		}
		if err != nil {
			fmt.Fprintf(rc.Err, "basename: write error: %v\n", err)
			return 1
		}
	}
	return 0
}

// base mirrors GNU basename: trailing slashes stripped first, the
// suffix removed only when it is a proper suffix (not the whole
// remaining name), "/" stays "/".
func base(name, suffix string) string {
	for len(name) > 1 && strings.HasSuffix(name, "/") {
		name = name[:len(name)-1]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 && len(name) > 1 {
		name = name[i+1:]
	}
	if suffix != "" && suffix != name && strings.HasSuffix(name, suffix) {
		name = name[:len(name)-len(suffix)]
	}
	return name
}
