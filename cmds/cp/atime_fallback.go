//go:build !linux && !darwin && !windows && !freebsd && !netbsd && !aix && !dragonfly && !openbsd && !solaris

package cpcmd

import (
	"os"
	"time"
)

// atime fails closed where no supported access-time field is wired up. POSIX
// -p requires the real atime; silently substituting mtime is a false success.
func atime(os.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
