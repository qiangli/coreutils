//go:build linux

package writecmd

import "errors"

// Linux keeps the live login-accounting database in /var/run/utmp (usually a
// symlink to /run/utmp) in glibc's `struct utmp` format. /var/log/wtmp is the
// historical LOG in the same format and is deliberately not consulted: it
// records sessions that have ended.
const defaultUtmpPath = "/var/run/utmp"

var activeUtmpLayout = layoutLinuxUtmp

const platformSupported = true

// Unused in production on Linux (platformSupported is a true constant), but
// defined so the refusal path has one spelling on every target.
var errPlatform = errors.New("terminal messaging is unavailable on this system")
