//go:build bashy_scratch && linux

package schedule

import "os"

// HostLoadAverage reads Linux's one-minute load average directly from procfs.
func HostLoadAverage() (float64, error) {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0, err
	}
	return parseProcLoadAverage(data)
}
