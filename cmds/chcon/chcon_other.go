//go:build !linux

package chconcmd

import (
	"fmt"
	"runtime"

	"github.com/qiangli/coreutils/tool"
)

func applyContext(rc *tool.RunContext, context string, files []string) int {
	fmt.Fprintf(rc.Err, "chcon: SELinux context changes are not supported on %s\n", runtime.GOOS)
	return 1
}
