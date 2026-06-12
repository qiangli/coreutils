// coreutils is the busybox-style multicall binary: invoke a tool as
// `coreutils <name> [args...]`, or symlink/rename the binary to a tool
// name and invoke it directly (argv[0] dispatch).
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/all"
)

func main() {
	name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	args := os.Args[1:]
	if name == "coreutils" {
		if len(args) == 0 || args[0] == "--list" {
			fmt.Println(strings.Join(tool.Names(), "\n"))
			return
		}
		name, args = args[0], args[1:]
	}
	t := tool.Lookup(name)
	if t == nil {
		fmt.Fprintf(os.Stderr, "coreutils: %q is not a supported command — see docs/commands.md for the plan (supported, planned, and deliberately-not-supported with reasons); 'coreutils --list' prints what this build ships\n", name)
		os.Exit(2)
	}
	dir, _ := os.Getwd()
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: dir,
		Env: os.Environ(),
		Stdio: tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
	os.Exit(t.Run(rc, args))
}
