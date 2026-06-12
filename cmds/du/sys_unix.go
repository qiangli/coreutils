//go:build unix

package ducmd

import (
	"os"
	"syscall"
)

type devIno struct{ dev, ino uint64 }

// usage is the file's disk usage in bytes (st_blocks × 512), or the
// apparent size with -b.
func (d *duRun) usage(fi os.FileInfo) int64 {
	if d.apparent {
		return fi.Size()
	}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks) * 512
	}
	return fi.Size()
}

// skipHardlink reports whether this non-directory was already counted
// through another hard link in this invocation (GNU counts multiply
// linked files once).
func (d *duRun) skipHardlink(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink <= 1 {
		return false
	}
	key := devIno{uint64(st.Dev), uint64(st.Ino)}
	if d.seen[key] {
		return true
	}
	d.seen[key] = true
	return false
}
