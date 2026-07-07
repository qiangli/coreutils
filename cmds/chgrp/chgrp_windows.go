//go:build !unix

package chgrpcmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

// apply on Windows fails loudly: there is no gid ownership model, and
// approximating one would change the documented meaning.
func apply(rc *tool.RunContext, _ string, _ []string, _, _, _, _, _, _, _, _, _ bool, _, _ int) int {
	fmt.Fprintf(rc.Err, "chgrp: not supported on windows: no POSIX uid/gid ownership exists on this platform\n")
	return 1
}

func parseFromSpec(string) (int, int, error) { return -1, -1, nil }

func statFile(*tool.RunContext, string) (*refFileInfo, error) {
	return nil, fmt.Errorf("not supported on windows")
}

type refFileInfo struct{}

func (*refFileInfo) gidStr() string { return "" }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chgrp: "+format+"\n", a...)
	return 1
}
