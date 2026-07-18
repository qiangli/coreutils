//go:build !darwin && !linux

package touchcmd

import (
	"errors"
	"os"
	"time"
)

// applyChtimesNoDeref has no symlink-aware primitive on this platform:
// os.Chtimes follows the link, which would silently retime the target
// instead of the link. Only a symlink operand is affected, so a plain file
// (where -h is a no-op) still works; a real symlink fails loudly rather
// than touching the wrong inode.
func applyChtimesNoDeref(path string, atime, mtime time.Time) error {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return errors.New("--no-dereference on a symbolic link is not supported on this platform")
	}
	return os.Chtimes(path, atime, mtime)
}
