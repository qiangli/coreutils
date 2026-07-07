//go:build !darwin && !linux

package touchcmd

import (
	"os"
	"time"
)

func applyChtimesNoDeref(path string, atime, mtime time.Time) error {
	return os.Chtimes(path, atime, mtime)
}
