//go:build linux

package uptimecmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func uptimeDuration() (time.Duration, error) {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(b))
	if len(fields) == 0 {
		return 0, fmt.Errorf("/proc/uptime is empty")
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("/proc/uptime: %v", err)
	}
	return time.Duration(secs * float64(time.Second)), nil
}

// loadAverages reads the three load averages from /proc/loadavg.
func loadAverages() (string, bool) {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(b))
	if len(fields) < 3 {
		return "", false
	}
	return strings.Join(fields[:3], ", "), true
}
