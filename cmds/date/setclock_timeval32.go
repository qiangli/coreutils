//go:build aix || darwin || netbsd

package datecmd

import (
	"time"

	"golang.org/x/sys/unix"
)

func setSystemClock(t time.Time) error {
	tv := unix.Timeval{Sec: t.Unix(), Usec: int32(t.Nanosecond() / 1_000)}
	return unix.Settimeofday(&tv)
}
