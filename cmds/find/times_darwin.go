//go:build darwin

package findcmd

import (
	"errors"
	"io/fs"
	"syscall"
	"time"
)

const (
	haveAtime = true
	haveCtime = true
)

func fileAtime(info fs.FileInfo) (time.Time, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, errors.New("no system stat data")
	}
	return time.Unix(st.Atimespec.Unix()), nil
}

func fileCtime(info fs.FileInfo) (time.Time, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, errors.New("no system stat data")
	}
	return time.Unix(st.Ctimespec.Unix()), nil
}
