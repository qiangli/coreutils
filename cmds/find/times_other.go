//go:build unix && !linux && !darwin

package findcmd

import (
	"errors"
	"io/fs"
	"time"
)

// The Stat_t access/change-time field names vary per platform;
// -atime/-ctime are wired only where a shipping target needs them
// and fail loudly at parse time elsewhere.
const (
	haveAtime = false
	haveCtime = false
)

func fileAtime(fs.FileInfo) (time.Time, error) {
	return time.Time{}, errors.New("-atime not supported on this platform")
}

func fileCtime(fs.FileInfo) (time.Time, error) {
	return time.Time{}, errors.New("-ctime not supported on this platform")
}
