//go:build windows

package findcmd

import (
	"errors"
	"io/fs"
	"syscall"
	"time"
)

// Windows records an access time but nothing with POSIX ctime
// (inode-change) semantics; -ctime fails loudly at parse time.
const (
	haveAtime = true
	haveCtime = false
)

func fileAtime(info fs.FileInfo) (time.Time, error) {
	st, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, errors.New("no system stat data")
	}
	return time.Unix(0, st.LastAccessTime.Nanoseconds()), nil
}

func fileCtime(fs.FileInfo) (time.Time, error) {
	return time.Time{}, errors.New("-ctime not supported on windows")
}
