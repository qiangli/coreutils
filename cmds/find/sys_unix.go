//go:build unix

package findcmd

import (
	"io/fs"
	"syscall"
)

// Predicates backed by the unix stat structure (-user/-group/-nouser/
// -nogroup/-links/-xdev) are supported wherever Sys() is a *Stat_t.
const (
	haveSysStat = true
	haveDev     = true
)

func statOf(info fs.FileInfo) (*syscall.Stat_t, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	return st, ok
}

func fileUID(info fs.FileInfo) (uint32, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint32(st.Uid), true
}

func fileGID(info fs.FileInfo) (uint32, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint32(st.Gid), true
}

func fileNlink(info fs.FileInfo) (uint64, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint64(st.Nlink), true
}

func fileDev(info fs.FileInfo) (uint64, bool) {
	st, ok := statOf(info)
	if !ok {
		return 0, false
	}
	return uint64(st.Dev), true
}
