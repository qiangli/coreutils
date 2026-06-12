//go:build windows

package uptimecmd

import (
	"time"

	"golang.org/x/sys/windows"
)

func uptimeDuration() (time.Duration, error) {
	// GetTickCount64: milliseconds since boot, no 49-day rollover.
	return windows.DurationSinceBoot(), nil
}

// loadAverages: Windows has no load-average concept; the field is
// omitted.
func loadAverages() (string, bool) {
	return "", false
}
