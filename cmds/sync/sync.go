// Package synccmd implements sync(1) per the GNU coreutils manual:
// synchronize cached writes to persistent storage. With FILE
// operands, fsync each named file; with none, sync the whole system.
//
// Implemented flags: -d/--data, -f/--file-system.
// Portions adapted from https://github.com/u-root/u-root cmds/core/sync/ (BSD-3-Clause).
package synccmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sync",
	Synopsis: "Synchronize cached writes to persistent storage. With FILEs, sync only them.",
	Usage:    "sync [OPTION] [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	flags := tool.NewFlags(cmd.Name)
	dataOnly := flags.BoolP("data", "d", false, "sync only file data, no unnecessary metadata")
	fileSystem := flags.BoolP("file-system", "f", false, "sync the file systems that contain the files")
	operands, code := tool.Parse(rc, cmd, flags, args)
	if code >= 0 {
		return code
	}

	useData := *dataOnly
	useFS := *fileSystem

	if len(operands) == 0 {
		if useFS {
			return tool.NotSupported(rc, cmd, "--file-system without file operands (syncfs requires a fd)")
		}
		if err := syncAll(); err != nil {
			fmt.Fprintf(rc.Err, "sync: %v\n", err)
			return 1
		}
		return 0
	}

	if useFS {
		return syncFSOperands(rc, operands)
	}

	status := 0
	for _, op := range operands {
		var err error
		if useData {
			err = syncFileData(rc.Path(op))
		} else {
			err = syncFile(rc.Path(op))
		}
		if err != nil {
			verb := "syncing"
			var pe *fs.PathError
			if errors.As(err, &pe) && pe.Op == "open" {
				verb = "opening"
			}
			fmt.Fprintf(rc.Err, "sync: error %s '%s': %v\n", verb, op, unwrapPath(err))
			status = 1
		}
	}
	return status
}

func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		var werr error
		if f, werr = os.OpenFile(path, os.O_WRONLY, 0); werr != nil {
			return err
		}
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
