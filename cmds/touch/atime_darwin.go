//go:build darwin

package touchcmd

import (
	"os"
	"syscall"
	"time"
)

// statAtime extracts the access time for -r on darwin.
func statAtime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Atimespec.Sec, st.Atimespec.Nsec)
	}
	return fi.ModTime()
}
