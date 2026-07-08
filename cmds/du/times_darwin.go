//go:build darwin

package ducmd

import (
	"os"
	"syscall"
	"time"
)

const haveSysTimes = true

// sysTime returns fi's access or status-change time (mtime is served
// portably from os.FileInfo and never reaches here).
func sysTime(fi os.FileInfo, sel timeSel) (time.Time, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, false
	}
	switch sel {
	case timeAtime:
		return time.Unix(st.Atimespec.Unix()), true
	case timeCtime:
		return time.Unix(st.Ctimespec.Unix()), true
	}
	return time.Time{}, false
}
