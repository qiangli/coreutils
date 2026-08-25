//go:build unix

package cpcmd

import (
	"os"
	"syscall"
)

// chownFn is a test seam: forcing an ownership-duplication failure
// without root is otherwise impossible, because chown to one's own
// uid/gid succeeds.
var chownFn = os.Chown

// preserveOwner applies the source uid/gid to dst and reports whether
// the duplication succeeded. Per POSIX -p (and the GNU manual), the
// failure itself is not an error and no diagnostic is required, but
// the caller must clear S_ISUID/S_ISGID from the duplicated mode.
func preserveOwner(dst string, fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return chownFn(dst, int(st.Uid), int(st.Gid)) == nil
}
