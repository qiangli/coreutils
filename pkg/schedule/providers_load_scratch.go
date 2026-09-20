//go:build bashy_scratch

package schedule

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func parseProcLoadAverage(data []byte) (float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("/proc/loadavg: missing one-minute load average")
	}
	load, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || load < 0 || math.IsInf(load, 0) || math.IsNaN(load) {
		return 0, fmt.Errorf("/proc/loadavg: invalid one-minute load average %q", fields[0])
	}
	return load, nil
}
