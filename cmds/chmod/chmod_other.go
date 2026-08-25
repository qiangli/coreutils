//go:build !unix

package chmodcmd

import (
	"fmt"
	"runtime"

	"github.com/qiangli/coreutils/tool"
)

// apply fails loudly where POSIX file mode bits are unavailable. Mapping MODE
// onto a read-only attribute or another host concept would silently change the
// meaning of the required interface.
func apply(rc *tool.RunContext, _ *modeChange, _ options) int {
	fmt.Fprintf(rc.Err, "chmod: not supported on %s: no POSIX file mode bits exist on this platform\n", runtime.GOOS)
	return 1
}
