//go:build bashy_scratch && !linux

package schedule

import "errors"

var errScratchLoadAverageUnsupported = errors.New("schedule: host load average is unsupported by bashy_scratch outside Linux")

// HostLoadAverage reports that the scratch profile has no load-average source
// on this platform.
func HostLoadAverage() (float64, error) {
	return 0, errScratchLoadAverageUnsupported
}
