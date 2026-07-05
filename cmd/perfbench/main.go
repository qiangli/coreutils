// Command perfbench is the DEV-ONLY host for the perfbench measurement harness
// (coreutils/cmds/perfbench). It links the full userland (cmds/all) so the
// in-process arm and present-status can reach every registered tool, plus the
// perfbench tool itself.
//
// It is NOT shipped: perfbench uses os/exec (the reference GNU arm) and must
// stay out of cmds/all, so the bare `coreutils` multicall binary and the `bash`
// drop-in never link it. This is the small dev binary the harness runs inside
// (locally for `list`/`gen`, and in bench.Containerfile for `run`/`conformance`
// where the GNU arm is present). See docs/coreutils-fidelity-perf-harness-spec.md.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/all"
	_ "github.com/qiangli/coreutils/cmds/perfbench"
)

func main() {
	wd, _ := os.Getwd()
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   wd,
		Env:   os.Environ(),
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
	}
	t := tool.Lookup("perfbench")
	if t == nil {
		fmt.Fprintln(os.Stderr, "perfbench: not registered")
		os.Exit(2)
	}
	os.Exit(t.Run(rc, os.Args[1:]))
}
