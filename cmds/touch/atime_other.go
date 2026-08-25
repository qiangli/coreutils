//go:build !linux && !darwin && !windows && !freebsd && !netbsd && !aix && !dragonfly && !openbsd && !solaris

package touchcmd

import (
	"os"
	"time"
)

// statAtime fails closed on platforms whose raw stat shape is not wired up.
// Returning mtime here would make `touch -a -r` report success while installing
// the wrong timestamp, which violates the command's upstream meaning.
func statAtime(os.FileInfo) (time.Time, bool) {
	return time.Time{}, false
}
