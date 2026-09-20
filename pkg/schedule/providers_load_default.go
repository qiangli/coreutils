//go:build !bashy_scratch

package schedule

import gopsload "github.com/shirou/gopsutil/v4/load"

// HostLoadAverage reads the host's one-minute load average without consulting
// command output or mutable scheduler state.
func HostLoadAverage() (float64, error) {
	avg, err := gopsload.Avg()
	if err != nil {
		return 0, err
	}
	return avg.Load1, nil
}
