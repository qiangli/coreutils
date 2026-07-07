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
	"os"
	"path/filepath"
	"strconv"
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
	null := fs.BoolP("null", "0", false, "end each output line with NUL, not newline")
	chdir := fs.StringP("chdir", "C", "", "change working directory before running COMMAND")
	argv0 := fs.StringP("argv0", "a", "", "pass ARGV0 as COMMAND's zeroth argument")
	splitStrings := fs.StringArrayP("split-string", "S", nil, "process and split S into separate arguments")
	envFiles := fs.StringArrayP("file", "f", nil, "read variables from a .env-style configuration file before other changes")
	ignoreSignals := fs.StringArray("ignore-signal", nil, "set handling of SIGNAL to ignore before running COMMAND")
	defaultSignals := fs.StringArray("default-signal", nil, "set handling of SIGNAL to default before running COMMAND")
	blockSignals := fs.StringArray("block-signal", nil, "block delivery of SIGNAL before running COMMAND")
	listSignalHandling := fs.Bool("list-signal-handling", false, "list non-default signal handling to stderr")
	debug := fs.BoolP("debug", "v", false, "print verbose information for each processing step")
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
	if *chdir != "" {
		st, err := os.Stat(rc.Path(*chdir))
		if err != nil {
			fmt.Fprintf(rc.Err, "env: cannot change directory to %q: %v\n", *chdir, tool.SysErr(err))
			return 125
		}
		if !st.IsDir() {
			fmt.Fprintf(rc.Err, "env: cannot change directory to %q: not a directory\n", *chdir)
			return 125
		}
		rc.Dir = filepath.Clean(rc.Path(*chdir))
	}
	for _, sigs := range [][]string{*ignoreSignals, *defaultSignals, *blockSignals} {
		for _, spec := range sigs {
			if err := validateSignals(spec); err != nil {
				return tool.UsageError(rc, cmd, "%v", err)
			}
		}
	}
	if *listSignalHandling {
		// In-process command execution is not available, and this
		// implementation has not altered signal state. GNU prints only
		// non-default dispositions, so the pure-Go data-mode output is empty.
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
	for _, name := range *envFiles {
		data, err := os.ReadFile(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "env: %s: %v\n", name, tool.SysErr(err))
			return 1
		}
		for _, raw := range envFileEntries(string(data)) {
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
	if len(*splitStrings) > 0 {
		var split []string
		for _, s := range *splitStrings {
			fields, err := splitString(s)
			if err != nil {
				return tool.UsageError(rc, cmd, "%v", err)
			}
			split = append(split, fields...)
		}
		operands = append(split, operands...)
	}
	if len(operands) > 0 {
		if *debug {
			fmt.Fprintf(rc.Err, "env: executing: %s\n", strings.Join(commandDebugArgv(*argv0, operands), " "))
		}
		return tool.NotSupported(rc, cmd, fmt.Sprintf("running a COMMAND (%q)", operands[0]))
	}
	if fs.Changed("argv0") {
		return tool.UsageError(rc, cmd, "--argv0 requires a COMMAND")
	}

	sep := "\n"
	if *null {
		sep = "\x00"
	}
	for _, e := range env {
		fmt.Fprintf(rc.Out, "%s%s", e.raw, sep)
	}
	return 0
}

func envFileEntries(data string) []string {
	if strings.Contains(data, "\x00") {
		parts := strings.Split(data, "\x00")
		if parts[len(parts)-1] == "" {
			parts = parts[:len(parts)-1]
		}
		return parts
	}
	var out []string
	for _, line := range strings.Split(data, "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func splitString(s string) ([]string, error) {
	var out []string
	var b strings.Builder
	quote := rune(0)
	esc := false
	inWord := false
	for _, r := range s {
		if esc {
			b.WriteRune(r)
			esc = false
			inWord = true
			continue
		}
		if r == '\\' {
			esc = true
			inWord = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			inWord = true
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			inWord = true
		case ' ', '\t', '\n':
			if inWord {
				out = append(out, b.String())
				b.Reset()
				inWord = false
			}
		default:
			b.WriteRune(r)
			inWord = true
		}
	}
	if esc {
		b.WriteRune('\\')
	}
	if quote != 0 {
		return nil, fmt.Errorf("no terminating quote in -S string")
	}
	if inWord {
		out = append(out, b.String())
	}
	return out, nil
}

func validateSignals(spec string) error {
	if spec == "" {
		return nil
	}
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			if n <= 0 {
				return fmt.Errorf("invalid signal %q", part)
			}
			continue
		}
		name := strings.TrimPrefix(strings.ToUpper(part), "SIG")
		if _, ok := knownSignals[name]; !ok {
			return fmt.Errorf("unknown signal %q", part)
		}
	}
	return nil
}

var knownSignals = func() map[string]struct{} {
	names := []string{"HUP", "INT", "QUIT", "ILL", "TRAP", "ABRT", "BUS", "FPE", "KILL", "USR1", "SEGV", "USR2", "PIPE", "ALRM", "TERM", "CHLD", "CONT", "STOP", "TSTP", "TTIN", "TTOU", "URG", "XCPU", "XFSZ", "VTALRM", "PROF", "WINCH", "IO", "SYS"}
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}()

func commandDebugArgv(argv0 string, operands []string) []string {
	out := append([]string{}, operands...)
	if argv0 != "" && len(out) > 0 {
		out[0] = argv0
	}
	return out
}
