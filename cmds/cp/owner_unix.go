//go:build unix

package cpcmd

import (
	"os"
	"syscall"
)

// preserveOwner applies the source uid/gid to dst, best effort: per
// the GNU manual, cp -p does not treat an ownership-preservation
// failure (e.g. not running as root) as an error.
func preserveOwner(dst string, fi os.FileInfo) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(dst, int(st.Uid), int(st.Gid))
	}
}
