//go:build !linux && !darwin

package synccmd

import (
	"fmt"
	"os"
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
	return f.Sync()
}

func syncFSOperands(rc *tool.RunContext, operands []string) int {
	for _, op := range operands {
		fmt.Fprintf(rc.Err, "sync: --file-system is not supported on this platform for '%s'\n", op)
	}
	return 1
}
