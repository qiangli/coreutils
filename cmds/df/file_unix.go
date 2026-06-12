//go:build linux || darwin

package dfcmd

import (
	"os"
	"strings"
	"syscall"
)

// mountForFile picks the mount containing path: prefer the mount
// point on the same device (longest one wins, for bind mounts), then
// fall back to the longest mount point that path-prefixes the path.
func mountForFile(path string, mounts []mountEntry) (int, bool) {
	best, bestLen := -1, -1
	if fi, err := os.Stat(path); err == nil {
		if dev, ok := devOf(fi); ok {
			for i, m := range mounts {
				mfi, merr := os.Stat(m.point)
				if merr != nil {
					continue
				}
				if md, mok := devOf(mfi); mok && md == dev && len(m.point) > bestLen {
					best, bestLen = i, len(m.point)
				}
			}
		}
	}
	if best >= 0 {
		return best, true
	}
	for i, m := range mounts {
		if pathHasPrefix(path, m.point) && len(m.point) > bestLen {
			best, bestLen = i, len(m.point)
		}
	}
	return best, best >= 0
}

func devOf(fi os.FileInfo) (uint64, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev), true
	}
	return 0, false
}

func pathHasPrefix(p, prefix string) bool {
	if prefix == "/" {
		return strings.HasPrefix(p, "/")
	}
	return p == prefix || strings.HasPrefix(p, prefix+"/")
}
