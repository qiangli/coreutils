//go:build unix

package paxcmd

import (
	"os"
	"syscall"
)

func identityOf(fi os.FileInfo) fileIdentity {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}
	}
	return fileIdentity{
		uid:   uint64(st.Uid),
		gid:   uint64(st.Gid),
		dev:   uint64(st.Dev),
		ino:   uint64(st.Ino),
		nlink: uint64(st.Nlink),
		ok:    true,
	}
}
