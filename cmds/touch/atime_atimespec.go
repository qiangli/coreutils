//go:build freebsd || netbsd

package touchcmd

import (
	"os"
	"syscall"
	"time"
)

func statAtime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(int64(st.Atimespec.Sec), int64(st.Atimespec.Nsec)), true
	}
	return time.Time{}, false
}
