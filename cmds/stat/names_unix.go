//go:build unix

package statcmd

import (
	"os"
	"os/user"
	"strconv"
	"sync"
)

var (
	idMu     sync.Mutex
	uidNames = map[uint32]string{}
	gidNames = map[uint32]string{}
)

// lookupUser resolves a uid via os/user; GNU stat prints "UNKNOWN"
// when the ID has no name.
func lookupUser(uid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := uidNames[uid]; ok {
		return n
	}
	n := "UNKNOWN"
	if u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10)); err == nil && u.Username != "" {
		n = u.Username
	}
	uidNames[uid] = n
	return n
}

func lookupGroup(gid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := gidNames[gid]; ok {
		return n
	}
	n := "UNKNOWN"
	if g, err := user.LookupGroupId(strconv.FormatUint(uint64(gid), 10)); err == nil && g.Name != "" {
		n = g.Name
	}
	gidNames[gid] = n
	return n
}

// fillFallback covers exotic filesystems whose Sys() is not a
// *syscall.Stat_t.
func fillFallback(m *fileMeta, fi os.FileInfo) {
	m.blocks = (fi.Size() + 511) / 512
	m.ioBlock = 4096
	m.nlink = 1
	m.uname, m.gname = "UNKNOWN", "UNKNOWN"
	m.atime, m.ctime = m.mtime, m.mtime
}
