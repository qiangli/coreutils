//go:build !linux && !darwin && !windows

package cpcmd

import (
	"os"
	"time"
)

// atime falls back to the modification time on platforms where the
// stat access-time field is not wired up here. Access-time fidelity
// is inherently weak (noatime/relatime mounts); this only affects -p.
func atime(fi os.FileInfo) time.Time {
	return fi.ModTime()
}
