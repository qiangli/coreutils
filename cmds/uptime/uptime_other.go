//go:build !linux && !darwin && !windows

package uptimecmd

import (
	"fmt"
	"runtime"
	"time"
)

func uptimeDuration() (time.Duration, error) {
	return 0, fmt.Errorf("not supported on %s: no uptime probe for this platform", runtime.GOOS)
}

func loadAverages() (string, bool) {
	return "", false
}
