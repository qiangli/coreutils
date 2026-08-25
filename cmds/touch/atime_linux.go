//go:build linux

package touchcmd

import (
	"os"
	"syscall"
	"time"
)

// statAtime extracts the access time for -r on linux. The boolean distinguishes
// a real access timestamp from platforms where Go exposes no supported stat
// field; callers must never silently substitute another timestamp.
func statAtime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atim.Sec, st.Atim.Nsec), true
	}
	return time.Time{}, false
}
