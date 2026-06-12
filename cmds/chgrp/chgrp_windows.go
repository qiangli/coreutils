//go:build !unix

package chgrpcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

// apply on Windows fails loudly: there is no gid ownership model, and
// approximating one would change the documented meaning.
func apply(rc *tool.RunContext, _ string, _ []string, _ bool) int {
	fmt.Fprintf(rc.Err, "chgrp: not supported on windows: no POSIX uid/gid ownership exists on this platform\n")
	return 1
}
