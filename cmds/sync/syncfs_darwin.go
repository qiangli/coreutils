//go:build darwin

package synccmd

import (
	"fmt"
	"os"

	"github.com/qiangli/coreutils/tool"
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
		fmt.Fprintf(rc.Err, "sync: syncing file system for '%s' is not supported on Darwin\n", op)
	}
	return 1
}
