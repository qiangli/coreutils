//go:build !unix

package chowncmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

// apply on Windows fails loudly: there is no uid/gid ownership model,
// and approximating one would change the documented meaning.
func apply(rc *tool.RunContext, _ string, _ []string, _, _, _, _, _, _, _, _, _ bool, _, _ int) int {
	fmt.Fprintf(rc.Err, "chown: not supported on windows: no POSIX uid/gid ownership exists on this platform\n")
	return 1
}

func parseSpec(string) (int, int, error) { return -1, -1, nil }

func statFile(*tool.RunContext, string) (*refFileInfo, error) {
	return nil, fmt.Errorf("not supported on windows")
}

type refFileInfo struct{}

func (*refFileInfo) ownerStr() string { return "" }

func statusError(rc *tool.RunContext, format string, a ...any) int {
	fmt.Fprintf(rc.Err, "chown: "+format+"\n", a...)
	return 1
}
