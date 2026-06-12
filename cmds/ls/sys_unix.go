//go:build unix

package lscmd

import (
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	idMu     sync.Mutex
	uidNames = map[uint32]string{}
	gidNames = map[uint32]string{}
)

// userName resolves a uid via os/user, falling back to the numeric ID
// when no name is known (GNU behavior).
func userName(uid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := uidNames[uid]; ok {
		return n
	}
	n := strconv.FormatUint(uint64(uid), 10)
	if u, err := user.LookupId(n); err == nil && u.Username != "" {
		n = u.Username
	}
	uidNames[uid] = n
	return n
}

func groupName(gid uint32) string {
	idMu.Lock()
	defer idMu.Unlock()
	if n, ok := gidNames[gid]; ok {
		return n
	}
	n := strconv.FormatUint(uint64(gid), 10)
	if g, err := user.LookupGroupId(n); err == nil && g.Name != "" {
		n = g.Name
	}
	gidNames[gid] = n
	return n
}

func sysOf(fi os.FileInfo, _ string) sysInfo {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return sysInfo{nlink: 1, blocks512: uint64((fi.Size() + 511) / 512)}
	}
	return sysInfo{
		nlink:     uint64(st.Nlink),
		owner:     userName(st.Uid),
		group:     groupName(st.Gid),
		blocks512: uint64(st.Blocks),
		rdevMajor: unix.Major(uint64(st.Rdev)),
		rdevMinor: unix.Minor(uint64(st.Rdev)),
	}
}

func inodeOf(fi os.FileInfo) uint64 {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return st.Ino
	}
	return 0
}
