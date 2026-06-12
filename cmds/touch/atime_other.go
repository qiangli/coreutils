//go:build !linux && !darwin && !windows

package touchcmd

import (
	"os"
	"time"
)

// statAtime falls back to the modification time on platforms where the
// raw stat shape is not wired up.
func statAtime(fi os.FileInfo) time.Time {
	return fi.ModTime()
}
