// Package synccmd implements sync(1) per the GNU coreutils manual:
// synchronize cached writes to persistent storage. With FILE
// operands, fsync each named file; with none, sync the whole system.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/sync/ (BSD-3-Clause).
// Changes: rewired to tool framework; per-file fsync via os.File.Sync
// (cross-platform); bare sync split behind build tags with a clear
// not-supported error on Windows; -d/-f not implemented (contract
// error names them).
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

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	flags := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, flags, args)
	if code >= 0 {
		return code
	}

	if len(operands) == 0 {
		// Whole-system sync: unix only (see sync_windows.go).
		if err := syncAll(); err != nil {
			fmt.Fprintf(rc.Err, "sync: %v\n", err)
			return 1
		}
		return 0
	}

	status := 0
	for _, op := range operands {
		if err := syncFile(rc.Path(op)); err != nil {
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

// syncFile opens path and fsyncs it. Like GNU sync, a file we cannot
// open for reading is retried write-only (fsync needs any open fd).
func syncFile(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		var werr error
		if f, werr = os.OpenFile(path, os.O_WRONLY, 0); werr != nil {
			return err // report the original open error
		}
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

// unwrapPath strips the *fs.PathError wrapper so the diagnostic reads
// "sync: error opening 'x': no such file or directory" rather than
// repeating the resolved path inside the message.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
