//go:build !linux && !darwin && !windows

package paxcmd

import (
	"errors"
	"time"
)

func defaultSetExtractedTimes(string, time.Time, bool, time.Time, bool, bool) error {
	return errors.New("preserving file times is not supported on this platform")
}
