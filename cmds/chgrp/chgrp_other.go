//go:build !unix

package chgrpcmd

import (
	"fmt"
	"runtime"

	"github.com/qiangli/coreutils/tool"
)

// apply fails loudly on platforms without POSIX uid/gid ownership;
// approximating ownership would change the documented meaning.
func apply(rc *tool.RunContext, _ string, _ options) int {
	fmt.Fprintf(rc.Err, "chgrp: not supported on %s: no POSIX uid/gid ownership exists on this platform\n", runtime.GOOS)
	return 1
}

func parseFromSpec(string) (int, int, error) { return -1, -1, nil }

func statFile(*tool.RunContext, string) (*refFileInfo, error) {
	return nil, fmt.Errorf("not supported on %s", runtime.GOOS)
}

type refFileInfo struct{}

func (*refFileInfo) ids() (uid, gid int) { return -1, -1 }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
