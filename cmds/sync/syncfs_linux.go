//go:build linux

package synccmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

func syncFileData(path string) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		var werr error
		if f, werr = os.OpenFile(path, os.O_WRONLY, 0); werr != nil {
			return err
		}
	}
	defer f.Close()
	return unix.Fdatasync(int(f.Fd()))
}

func syncFSOperands(rc *tool.RunContext, operands []string) int {
	status := 0
	for _, op := range operands {
		path := rc.Path(op)
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintf(rc.Err, "sync: error opening '%s': %v\n", op, err)
			status = 1
			continue
		}
		err = unix.Syncfs(int(f.Fd()))
		f.Close()
		if err != nil {
			fmt.Fprintf(rc.Err, "sync: error syncing file system for '%s': %v\n", op, err)
			status = 1
		}
	}
	return status
}
