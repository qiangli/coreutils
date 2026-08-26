// Package envcmd implements env(1) per the GNU coreutils manual:
// print the environment, optionally modified by -i, -u NAME, and
// NAME=VALUE assignments, or run a command in that environment.
//
// Running COMMAND is the upstream-documented purpose of env, so this is
// one of the command wrappers exempted from the no-shell-out rule (see
// docs/commands.md): the utility is invoked directly through exec, never
// by concatenating a string for a shell. Exit status follows GNU: 126 if
// COMMAND is found but cannot be invoked, 127 if it cannot be found, 125
// if env itself fails, otherwise COMMAND's own status (128+N when it is
// killed by a signal). Usage errors exit 2 per the repo-wide convention,
// where GNU uses 125.
//
// Portions adapted from https://github.com/guonaihong/coreutils env/env.go (Apache-2.0).
// Changes: rewired to the tool framework over RunContext.Env (no
// os.Environ/os.Setenv globals); direct COMMAND execution; '-' first-
// operand alias for -i; -u made repeatable.
package envcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// A RunContext can be reused by an embedder. Never let a prior signal
	// outcome leak into this invocation when the next COMMAND exits normally.
	rc.ExitSignal = 0
	fs := tool.NewFlags(cmd.Name)
	ignore := fs.BoolP("ignore-environment", "i", false, "start with an empty environment")
	unset := fs.StringArrayP("unset", "u", nil, "remove variable from the environment")
	null := fs.BoolP("null", "0", false, "end each output line with NUL, not newline")
	chdir := fs.StringP("chdir", "C", "", "change working directory before running COMMAND")
	argv0 := fs.StringP("argv0", "a", "", "pass ARGV0 as COMMAND's zeroth argument")
	splitStrings := fs.StringArrayP("split-string", "S", nil, "process and split S into separate arguments")
	envFiles := fs.StringArrayP("file", "f", nil, "read variables from a .env-style configuration file before other changes")
	ignoreSignals := fs.StringArray("ignore-signal", nil, "set handling of SIGNAL to ignore before running COMMAND")
	fs.Lookup("ignore-signal").NoOptDefVal = "all"
	debug := fs.BoolP("debug", "v", false, "print verbose information for each processing step")
	// The first non-option operand begins NAME=VALUE processing and then
	// COMMAND. Options belonging to COMMAND (for example, sh -c) must not be
	// parsed as env options.
	fs.SetInterspersed(false)
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
	signals, err := commandSignals(*ignoreSignals)
	if err != nil {
		return tool.UsageError(rc, cmd, "%v", err)
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
	// Leading operands containing '=' are assignments; the first operand
	// without '=' starts COMMAND. Split-string fields participate in the same
	// assignment scan.
	for len(operands) > 0 && strings.Contains(operands[0], "=") {
		set(operands[0])
		operands = operands[1:]
	}
	if len(operands) > 0 {
		if *debug {
			fmt.Fprintf(rc.Err, "env: executing: %s\n", strings.Join(commandDebugArgv(*argv0, operands), " "))
		}
		rawEnv := make([]string, len(env))
		for i, e := range env {
			rawEnv[i] = e.raw
		}
		return runCommand(rc, operands, rawEnv, *argv0, signals)
	}
	if fs.Changed("argv0") {
		return tool.UsageError(rc, cmd, "--argv0 requires a COMMAND")
	}

	sep := "\n"
	if *null {
		sep = "\x00"
	}
	for _, e := range env {
		if _, err := fmt.Fprintf(rc.Out, "%s%s", e.raw, sep); err != nil {
			fmt.Fprintf(rc.Err, "env: write error: %v\n", err)
			return 1
		}
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

func runCommand(rc *tool.RunContext, argv, env []string, argv0 string, signals []os.Signal) int {
	path, found := lookCommand(rc, argv[0], env)
	if !found {
		fmt.Fprintf(rc.Err, "env: %s: No such file or directory\n", argv[0])
		return 127
	}

	newCmd := func(name string, args []string) *exec.Cmd {
		c := exec.CommandContext(commandContext(rc), name, args...)
		c.Dir = rc.Dir
		c.Env = env
		c.Stdin = rc.In
		c.Stdout = rc.Out
		c.Stderr = rc.Err
		return c
	}

	// argv is passed through element-for-element: no shell, no quoting, no
	// concatenation, so metacharacters in an argument reach COMMAND as data.
	c := newCmd(path, argv[1:])
	if argv0 != "" {
		c.Args[0] = argv0
	}

	restoreSignals := ignoreForCommandStart(signals)
	err := c.Start()
	if err != nil && isExecFormatError(err) && scriptInterpreter != "" {
		// Historical exec() behavior, relied on by the GNU baseline via
		// glibc's execvp: a file that the kernel does not recognize as an
		// executable image — a script with no "#!" interpreter line — is
		// retried through the system shell, with the shell as argv[0] and
		// the resolved path inserted ahead of the original arguments. Per
		// glibc's own fallback, argv[0] is always the resolved path here,
		// so an --argv0 override does not survive the retry.
		c = newCmd(scriptInterpreter, append([]string{path}, argv[1:]...))
		err = c.Start()
	}
	restoreSignals()
	if err != nil {
		fmt.Fprintf(rc.Err, "env: %s: %v\n", argv[0], err)
		if errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err) {
			return 127
		}
		return 126
	}
	if err := c.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			if code := ee.ExitCode(); code >= 0 {
				return code
			}
			// COMMAND was killed by a signal. Report the safe 128+N exit
			// status to every caller, and record the raw signal number so a
			// standalone process boundary can re-raise it and inherit
			// COMMAND's exact wait status (env self-replaces via execve
			// upstream; we fork/wait, so this is how the signal propagates).
			sig, status := commandSignalOutcome(ee.ProcessState)
			rc.ExitSignal = sig
			return status
		}
		fmt.Fprintf(rc.Err, "env: %s: %v\n", argv[0], err)
		return 126
	}
	return 0
}

func lookCommand(rc *tool.RunContext, name string, env []string) (string, bool) {
	if strings.ContainsAny(name, `/\`) {
		path := rc.ResolveExecutable(name)
		_, err := os.Stat(path)
		return path, err == nil || !os.IsNotExist(err)
	}

	pathValue, present := envValue(env, "PATH")
	if !present {
		pathValue = defaultCommandPath()
	}
	var firstFound string
	for _, dir := range commandSearchPath(pathValue) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		if !filepath.IsAbs(candidate) {
			candidate = rc.Path(candidate)
		}
		candidate = resolveCommandCandidate(rc, candidate)
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if firstFound == "" {
			firstFound = candidate
		}
		if info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
			return candidate, true
		}
	}
	return firstFound, firstFound != ""
}

// commandSearchPath splits a PATH value into search prefixes. POSIX makes a
// zero-length prefix mean the working directory, and a PATH that is entirely
// empty is one zero-length prefix — not "nowhere to look" — so an empty PATH
// searches the working directory only. filepath.SplitList("") returns no
// elements at all, hence the explicit case.
func commandSearchPath(value string) []string {
	dirs := filepath.SplitList(value)
	if len(dirs) == 0 {
		return []string{""}
	}
	return dirs
}

// commandContext ties COMMAND's lifetime to the invocation context, so a
// host cancelling the tool kills the child instead of leaking it. Hosts that
// leave Ctx nil get an uncancellable child rather than a panic.
func commandContext(rc *tool.RunContext) context.Context {
	if rc.Ctx == nil {
		return context.Background()
	}
	return rc.Ctx
}

func commandDebugArgv(argv0 string, operands []string) []string {
	out := append([]string{}, operands...)
	if argv0 != "" && len(out) > 0 {
		out[0] = argv0
	}
	return out
}

func envValue(env []string, name string) (string, bool) {
	prefix := name + "="
	for i := len(env) - 1; i >= 0; i-- {
		if strings.HasPrefix(env[i], prefix) {
			return env[i][len(prefix):], true
		}
	}
	return "", false
}
