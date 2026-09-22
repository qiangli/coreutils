// Package multicall is the busybox-style dispatch shared by every binary
// that fronts the coreutils tool registry: the standalone `coreutils`
// binary, a symlink/rename to a tool name (argv[0] dispatch), and the
// AgentOS `bashy` bootstrapper which also offers `bashy <tool> …`.
//
// Keeping the dispatch here (not inlined in a main package) lets bashy
// and any other host reuse the exact same name-resolution and
// RunContext-construction behavior the standalone binary has.
//
// Callers must blank-import the tool sets they want available
// (e.g. github.com/qiangli/coreutils/cmds/all) so init() registration
// has run before Dispatch is called.
package multicall

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// Resolve decodes (argv0, args, selfNames) into the tool name and its
// arguments. When the binary is invoked under one of its own selfNames
// (e.g. "coreutils" or "bashy"), the first operand is the tool name;
// otherwise argv0 itself is the tool name (symlink/rename dispatch).
//
// The returned listOnly is true when the multicall front-end was asked to
// list available tools rather than run one (no operand, or "--list").
//
// The basename is taken as a login shell takes it: ONE leading dash is
// stripped (bash's `exec -l printenv`, and every login(1)-style launcher,
// spell argv[0] as "-printenv" — see execve(2)/bash(1) "-l"), and a
// Windows ".exe" suffix is dropped case-insensitively (the filesystem
// that put it there is case-insensitive too, so LS.EXE and ls.exe are the
// same program).
func Resolve(argv0 string, args []string, selfNames ...string) (name string, toolArgs []string, listOnly bool) {
	base := commandName(argv0)
	for _, self := range selfNames {
		if base == self {
			if len(args) == 0 || args[0] == "--list" {
				return "", nil, true
			}
			return args[0], args[1:], false
		}
	}
	return base, args, false
}

// commandName is the tool name argv0 stands for: its basename, without
// one leading dash and without a case-insensitive ".exe" suffix. A name
// that is only a dash, or only ".exe", is left alone rather than reduced
// to nothing.
func commandName(argv0 string) string {
	base := filepath.Base(argv0)
	if len(base) > 1 && base[0] == '-' {
		base = base[1:]
	}
	if n := len(base) - len(".exe"); n > 0 && strings.EqualFold(base[n:], ".exe") {
		base = base[:n]
	}
	return base
}

// Dispatch runs the named tool against rc and returns its exit code. It
// returns 2 with a diagnostic on rc.Err for an unknown tool, matching the
// standalone binary's behavior.
func Dispatch(rc *tool.RunContext, name string, args []string) int {
	t := tool.Lookup(name)
	if t == nil {
		fmt.Fprintf(rc.Err, "%s: %q is not a supported command — see docs/commands.md for the plan (supported, planned, and deliberately-not-supported with reasons); '--list' prints what this build ships\n", filepath.Base(name), name)
		return 2
	}
	return t.Run(rc, args)
}

// processRunContext builds the RunContext the standalone process runs
// tools against: real stdio, real environment, and the process working
// directory — declared as such (DirIsProcessCwd) so relative operands
// whose joined absolute form would overrun the platform path-length
// limit stay relative and resolve through the kernel's own cwd lookup,
// exactly as they do for an execve'd GNU tool.
func processRunContext() *tool.RunContext {
	dir, _ := os.Getwd()
	return &tool.RunContext{
		Ctx:              context.Background(),
		Dir:              dir,
		DirIsProcessCwd:  true,
		DedicatedProcess: true,
		InvocationName:   os.Args[0],
		Env:              os.Environ(),
		FS:               tool.NewLocalFS(),
		SIGPIPEIgnored:   inheritedSIGPIPEWasIgnored(),
		Stdio: tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
}

// Main is a complete multicall entrypoint: it reads the process argv, env,
// and cwd, resolves the tool, and exits with its status. selfNames are the
// front-end binary names under which the first operand is the tool name
// (e.g. "coreutils", "bashy").
//
// Main owns its process (it calls os.Exit), so it also acts as the standalone
// process boundary: when a command wrapper reports a signal-terminated COMMAND
// via RunContext.ExitSignal, Main re-raises that signal on itself
// (TerminateBySignal) so its wait status matches COMMAND's. Embedded hosts that
// stay resident must NOT call Main; they run tools through Dispatch or
// tool.Tool.Run, which never signal the process.
func Main(selfNames ...string) {
	preserveInheritedSignalDispositions()
	name, args, listOnly := Resolve(os.Args[0], os.Args[1:], selfNames...)
	if listOnly {
		fmt.Println(strings.Join(tool.Names(), "\n"))
		return
	}
	rc := processRunContext()
	code := Dispatch(rc, name, args)
	// Standalone process boundary: if a wrapped COMMAND was killed by a
	// signal, re-raise it on ourselves so our wait status matches COMMAND's,
	// exactly as an execve-replacing env would. On a terminating signal this
	// does not return; otherwise fall through to the normal exit.
	if rc.ExitSignal != 0 {
		TerminateBySignal(rc.ExitSignal)
	}
	os.Exit(code)
}
