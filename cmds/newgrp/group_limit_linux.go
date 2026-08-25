//go:build linux

package newgrpcmd

import (
	"os"
	"strconv"
	"strings"
)

func maximumSupplementaryGroups() int {
	data, err := os.ReadFile("/proc/sys/kernel/ngroups_max")
	if err != nil {
		return 0
	}
	limit, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || limit <= 0 {
		return 0
	}
	return limit
}
