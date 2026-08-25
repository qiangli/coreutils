//go:build windows

package paxcmd

import (
	"errors"
	"os"
	"time"
)

func defaultSetExtractedTimes(path string, atime time.Time, setA bool, mtime time.Time, setM, symlink bool) error {
	if symlink {
		return errors.New("preserving symbolic-link times is not supported on windows")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !setA {
		var ok bool
		atime, ok = sourceAccessTime(fi)
		if !ok {
			return errors.New("current access time is unavailable on windows")
		}
	}
	if !setM {
		mtime = fi.ModTime()
	}
	return os.Chtimes(path, atime, mtime)
}
