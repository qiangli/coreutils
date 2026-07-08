//go:build !linux && !darwin

package synccmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

// syncFileData: no fdatasync here (this arm includes windows, where
// FlushFileBuffers also needs a WRITABLE handle) — delegate to syncFile,
// which picks the right open mode per platform.
func syncFileData(path string) error {
	return syncFile(path)
}

func syncFSOperands(rc *tool.RunContext, operands []string) int {
	for _, op := range operands {
		fmt.Fprintf(rc.Err, "sync: --file-system is not supported on this platform for '%s'\n", op)
	}
	return 1
}
