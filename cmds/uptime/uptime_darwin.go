//go:build darwin

package uptimecmd

import (
	"time"

	"golang.org/x/sys/unix"
)

func uptimeDuration() (time.Duration, error) {
	tv, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0, err
	}
	boot := time.Unix(tv.Sec, int64(tv.Usec)*1000)
	return time.Since(boot), nil
}

// loadAverages: omitted on darwin (load averages print on Linux
// only; see the package note).
func loadAverages() (string, bool) {
	return "", false
}
