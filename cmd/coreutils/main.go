// coreutils is the busybox-style multicall binary over the REQUIRED set
// (POSIX-required ∪ GNU coreutils — cmds/all): invoke a tool as
// `coreutils <name> [args...]`, or symlink/rename the binary to a tool
// name and invoke it directly (argv[0] dispatch).
//
// This is the certified binary. It links coreutils only: no yoke package,
// no MCP front (that is `yoke mcp` / `bashy mcp`), nothing an agent surface
// needs and a POSIX utility does not.
package main

import (
	"fmt"
	"os"

	"github.com/qiangli/bash/interp/ownedexec"
	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/pkg/schedule"

	_ "github.com/qiangli/coreutils/cmds/all"
)

func main() {
	if err := ownedexec.Adopt(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(126)
	}
	name, args, listOnly := multicall.Resolve(os.Args[0], os.Args[1:], "coreutils")
	if !listOnly && name == "schedule" {
		cmd := schedule.NewScheduleCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		return
	}
	multicall.Main("coreutils")
}
