//go:build windows

package morecmd

import (
	"fmt"

	"github.com/qiangli/coreutils/tool"
)

var openControllingTTY = func(_ *tool.RunContext) (*ttyChannel, error) {
	return nil, fmt.Errorf("interactive terminal mode is not supported on Windows")
}
