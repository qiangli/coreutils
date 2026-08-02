//go:build linux

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
	return time.Unix(st.Atim.Unix()), nil
}

func fileCtime(info fs.FileInfo) (time.Time, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, errors.New("no system stat data")
	}
	return time.Unix(st.Ctim.Unix()), nil
}
