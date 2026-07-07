//go:build linux

package nproccmd

import (
	"os"
	"strconv"
	"strings"
)

func cgroupQuota() int {
	cg, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return 0
	}
	line := strings.SplitN(string(cg), "\n", 2)[0]
	parts := strings.Split(line, ":")
	if len(parts) < 3 {
		return 0
	}
	data, err := os.ReadFile("/sys/fs/cgroup" + parts[2] + "/cpu.max")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) < 2 || fields[0] == "max" {
		return 0
	}
	quota, err1 := strconv.ParseUint(fields[0], 10, 64)
	period, err2 := strconv.ParseUint(fields[1], 10, 64)
	if err1 != nil || err2 != nil || period == 0 {
		return 0
	}
	return int((quota + period/2) / period)
}
