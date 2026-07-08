//go:build unix && !darwin && !linux

package ducmd

import (
	"os"
	"time"
)

// The non-mtime --time fields are not wired up on this platform, so
// --time=atime/ctime fails loudly at parse time (contract: a clear
// error, never a silent mtime approximation).
const haveSysTimes = false

func sysTime(_ os.FileInfo, _ timeSel) (time.Time, bool) {
	return time.Time{}, false
}
