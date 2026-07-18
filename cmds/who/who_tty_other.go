//go:build !linux && !darwin && !windows

package whocmd

import (
	"os"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func stdinTTY(_ *tool.RunContext) (string, bool) {
	return "", false
}

func accessTime(_ string) (time.Time, bool) {
	return time.Time{}, false
}
