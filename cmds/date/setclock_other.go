//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package datecmd

import (
	"fmt"
	"runtime"
	"time"
)

func setSystemClock(time.Time) error {
	return fmt.Errorf("setting the system clock is unavailable on %s", runtime.GOOS)
}
