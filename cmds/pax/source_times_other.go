//go:build !linux && !darwin && !windows

package paxcmd

import (
	"errors"
	"os"
	"time"
)

func sourceAccessTime(os.FileInfo) (time.Time, bool) { return time.Time{}, false }

func restoreSourceTimes(string, time.Time, time.Time, bool) error {
	return errors.New("source access-time restoration is not supported on this platform")
}
