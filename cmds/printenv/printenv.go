// Package printenvcmd implements printenv(1) per the GNU coreutils
// manual: print all environment variables, or the values of the named
// VARIABLEs. Exit status is 1 when at least one named variable is not
// set.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/printenv/printenv.go (BSD-3-Clause).
// Changes: rewired to the tool framework over RunContext.Env (no
// os.Environ); GNU exit status for missing variables.
package printenvcmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "printenv",
	Synopsis: "Print the values of the specified environment VARIABLE(s), or all of them.",
	Usage:    "printenv [OPTION]... [VARIABLE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	null := fs.BoolP("null", "0", false, "end each output line with NUL, not newline")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	lineTerm := "\n"
	if *null {
		lineTerm = "\x00"
	}

	if len(operands) == 0 {
		for _, raw := range rc.Env {
			fmt.Fprintf(rc.Out, "%s%s", raw, lineTerm)
		}
		return 0
	}

	status := 0
	for _, name := range operands {
		val, ok := lookup(rc.Env, name)
		if !ok {
			status = 1
			continue
		}
		fmt.Fprintf(rc.Out, "%s%s", val, lineTerm)
	}
	return status
}

// lookup distinguishes set-but-empty (printed as a blank line) from
// unset (nothing printed, exit 1). Last assignment wins, matching
// RunContext.Getenv.
func lookup(env []string, name string) (string, bool) {
	if name == "" || strings.Contains(name, "=") {
		return "", false
	}
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}
