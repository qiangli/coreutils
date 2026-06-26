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
func Resolve(argv0 string, args []string, selfNames ...string) (name string, toolArgs []string, listOnly bool) {
	base := strings.TrimSuffix(filepath.Base(argv0), ".exe")
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

// Main is a complete multicall entrypoint: it reads the process argv, env,
// and cwd, resolves the tool, and exits with its status. selfNames are the
// front-end binary names under which the first operand is the tool name
// (e.g. "coreutils", "bashy").
func Main(selfNames ...string) {
	name, args, listOnly := Resolve(os.Args[0], os.Args[1:], selfNames...)
	if listOnly {
		fmt.Println(strings.Join(tool.Names(), "\n"))
		return
	}
	dir, _ := os.Getwd()
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: os.Environ(),
		FS:  tool.NewLocalFS(),
		Stdio: tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
	os.Exit(Dispatch(rc, name, args))
}
