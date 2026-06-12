// Package envcmd implements env(1) per the GNU coreutils manual:
// print the environment, optionally modified by -i, -u NAME, and
// NAME=VALUE assignments.
//
// Running a COMMAND operand is documented-but-unsupported (process
// execution; revisits with the sh ExecHandler) and fails with the
// clear contract error.
//
// Portions adapted from https://github.com/guonaihong/coreutils env/env.go (Apache-2.0).
// Changes: rewired to the tool framework over RunContext.Env (no
// os.Environ/os.Setenv globals); COMMAND execution removed per repo
// rules; '-' first-operand alias for -i; -u made repeatable.
package envcmd

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "env",
	Synopsis: "Set each NAME to VALUE in the environment and print the resulting environment.",
	Usage:    "env [OPTION]... [-] [NAME=VALUE]... [COMMAND [ARG]...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

// entry preserves the raw "NAME=VALUE" string so undecorated environ
// entries round-trip byte-for-byte.
type entry struct {
	name string
	raw  string
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	ignore := fs.BoolP("ignore-environment", "i", false, "start with an empty environment")
	unset := fs.StringArrayP("unset", "u", nil, "remove variable from the environment")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	// "env - NAME=VALUE..." — a lone '-' first operand is the
	// documented obsolete synonym for -i.
	if len(operands) > 0 && operands[0] == "-" {
		*ignore = true
		operands = operands[1:]
	}

	// Build the environment in environ order, like a real process
	// would by repeated setenv: first occurrence keeps its slot, a
	// later assignment to the same NAME updates it in place.
	var env []entry
	idx := map[string]int{}
	set := func(raw string) {
		name, _, ok := strings.Cut(raw, "=")
		if !ok {
			name = raw
		}
		if i, dup := idx[name]; dup {
			env[i].raw = raw
			return
		}
		idx[name] = len(env)
		env = append(env, entry{name: name, raw: raw})
	}
	if !*ignore {
		for _, raw := range rc.Env {
			set(raw)
		}
	}

	for _, name := range *unset {
		if name == "" || strings.Contains(name, "=") {
			return tool.UsageError(rc, cmd, "cannot unset %q", name)
		}
		if i, ok := idx[name]; ok {
			env = append(env[:i], env[i+1:]...)
			delete(idx, name)
			for n, j := range idx {
				if j > i {
					idx[n] = j - 1
				}
			}
		}
	}

	// Leading operands containing '=' are assignments; the first
	// operand without '=' starts a COMMAND, which is not supported.
	for len(operands) > 0 && strings.Contains(operands[0], "=") {
		set(operands[0])
		operands = operands[1:]
	}
	if len(operands) > 0 {
		return tool.NotSupported(rc, cmd, fmt.Sprintf("running a COMMAND (%q)", operands[0]))
	}

	for _, e := range env {
		fmt.Fprintf(rc.Out, "%s\n", e.raw)
	}
	return 0
}
