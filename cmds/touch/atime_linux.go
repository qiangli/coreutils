//go:build linux

package touchcmd

import (
	"os"
	"syscall"
	"time"
)

// statAtime extracts the access time for -r on linux.
func statAtime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atim.Sec, st.Atim.Nsec)
	}
	return fi.ModTime()
}
