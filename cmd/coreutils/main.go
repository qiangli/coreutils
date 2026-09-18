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
	"github.com/qiangli/coreutils/multicall"

	_ "github.com/qiangli/coreutils/cmds/all"
)

func main() {
	multicall.Main("coreutils")
}
