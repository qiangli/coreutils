//go:build darwin

package writecmd

import "errors"

// Darwin has no utmp: the live database is /var/run/utmpx in `struct utmpx`
// format, whose field order differs from Linux's utmp (ut_user comes first,
// not ut_type). See layoutDarwinUtmpx for the byte-level layout.
const defaultUtmpPath = "/var/run/utmpx"

var activeUtmpLayout = layoutDarwinUtmpx

const platformSupported = true

// Unused in production on Darwin (platformSupported is a true constant), but
// defined so the refusal path has one spelling on every target.
var errPlatform = errors.New("terminal messaging is unavailable on this system")
