//go:build darwin

package writecmd

import "errors"

// Darwin has no utmp: the live database is /var/run/utmpx in `struct utmpx`
// format, whose field order differs from Linux's utmp (ut_user comes first,
// not ut_type). See layoutDarwinUtmpx for the byte-level layout.
const defaultUtmpPath = "/var/run/utmpx"

var activeUtmpLayout = utmpLayout{}

// Darwin exposes no procfs-style, race-resistant PID-to-controlling-terminal
// association and this pure-Go implementation has no sysctl provider for it.
// Accepting any live PID would authenticate stale or unrelated utmpx records,
// so fail closed until that association can be proved.
const platformSupported = false

// The refusal path names the missing authentication capability explicitly.
var errPlatform = errors.New("terminal messaging requires PID-to-terminal authentication unavailable on Darwin")
