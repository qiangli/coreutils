//go:build !unix

package chmodcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

// apply on Windows fails loudly: there are no POSIX mode bits, and
// mapping MODE onto the read-only attribute would silently change the
// documented meaning of every flag.
func apply(rc *tool.RunContext, _ *modeChange, _ []string, _, _, _, _, _, _, _, _, _ bool) int {
	fmt.Fprintf(rc.Err, "chmod: not supported on windows: no POSIX file mode bits exist on this platform\n")
	return 1
}
