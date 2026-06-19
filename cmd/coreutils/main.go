// coreutils is the busybox-style multicall binary: invoke a tool as
// `coreutils <name> [args...]`, or symlink/rename the binary to a tool
// name and invoke it directly (argv[0] dispatch).
//
// The reserved front-end subcommand `coreutils mcp` starts the Model
// Context Protocol server over stdio instead of dispatching a tool.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/mcp"
	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"

	_ "github.com/qiangli/coreutils/cmds/all"
)

func main() {
	// Only the `coreutils mcp` front-end form starts the server; when the
	// binary is symlinked to a tool name, `mcp` is just that tool's operand.
	base := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe")
	if base == "coreutils" && len(os.Args) > 1 && os.Args[1] == "mcp" {
		if err := mcp.ServeStdio(context.Background(), "coreutils", tool.Version); err != nil {
			fmt.Fprintln(os.Stderr, "coreutils mcp:", err)
			os.Exit(1)
		}
		return
	}
	multicall.Main("coreutils")
}
