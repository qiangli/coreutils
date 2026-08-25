//go:build unix

package mvcmd

import (
	"os"
	"syscall"
)

// preserveOwner applies the source uid/gid to dst and returns an error
// if it fails (useful for dropping setuid/setgid).
func preserveOwner(dst string, fi os.FileInfo) error {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return os.Chown(dst, int(st.Uid), int(st.Gid))
	}
	return nil
}

func preserveFileOwner(dst *os.File, fi os.FileInfo) error {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return dst.Chown(int(st.Uid), int(st.Gid))
	}
	return nil
}

func preserveLinkOwner(dst string, fi os.FileInfo) error {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return os.Lchown(dst, int(st.Uid), int(st.Gid))
	}
	return nil
}
