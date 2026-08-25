//go:build linux

package writecmd

import "errors"

// Linux keeps the live login-accounting database in /var/run/utmp (usually a
// symlink to /run/utmp) in glibc's `struct utmp` format. /var/log/wtmp is the
// historical LOG in the same format and is deliberately not consulted: it
// records sessions that have ended.
const defaultUtmpPath = "/var/run/utmp"

var errPlatform = errors.New("terminal messaging is unavailable for this Linux architecture")
