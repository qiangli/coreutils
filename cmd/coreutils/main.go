// coreutils is the busybox-style multicall binary: invoke a tool as
// `coreutils <name> [args...]`, or symlink/rename the binary to a tool
// name and invoke it directly (argv[0] dispatch).
package main

import (
	"github.com/qiangli/coreutils/multicall"

	_ "github.com/qiangli/coreutils/cmds/all"
)

func main() {
	multicall.Main("coreutils")
}
