//go:build dragonfly || freebsd || linux || openbsd

package datecmd

import (
	"time"

	"golang.org/x/sys/unix"
)

// setSystemClock is the production XSI clock mutation boundary. Tests invoke
// runWithClock with a recorder and never alter the host clock.
func setSystemClock(t time.Time) error {
	// Construct from Unix seconds rather than UnixNano so the four-digit XSI
	// year field is not artificially limited to Go's nanosecond epoch range.
	tv := unix.Timeval{Sec: t.Unix(), Usec: int64(t.Nanosecond() / 1_000)}
	return unix.Settimeofday(&tv)
}
