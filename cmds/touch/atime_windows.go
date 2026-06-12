//go:build windows

package touchcmd

import (
	"os"
	"syscall"
	"time"
)

// statAtime extracts the access time for -r on windows.
func statAtime(fi os.FileInfo) time.Time {
	if st, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, st.LastAccessTime.Nanoseconds())
	}
	return fi.ModTime()
}
