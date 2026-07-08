//go:build darwin

package lscmd

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// sysTime returns fi's access, status-change, or birth time (mtime is
// served portably from os.FileInfo and never reaches here).
func sysTime(fi os.FileInfo, _ string, sel timeSel) (time.Time, error) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, fmt.Errorf("no system stat data")
	}
	switch sel {
	case selAtime:
		return time.Unix(st.Atimespec.Unix()), nil
	case selCtime:
		return time.Unix(st.Ctimespec.Unix()), nil
	case selBirth:
		return time.Unix(st.Birthtimespec.Unix()), nil
	}
	return fi.ModTime(), nil
}
