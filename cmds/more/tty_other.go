//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package morecmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

var openControllingTTY = func(_ *tool.RunContext) (*ttyChannel, error) {
	return nil, fmt.Errorf("interactive terminal mode is not supported on this platform")
}
