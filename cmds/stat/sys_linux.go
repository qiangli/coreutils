//go:build linux

package statcmd

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func fillSys(m *fileMeta, _ string, fi os.FileInfo) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		fillFallback(m, fi)
		return
	}
	m.blocks = st.Blocks
	m.ioBlock = int64(st.Blksize)
	m.uid, m.gid = st.Uid, st.Gid
	m.uname, m.gname = lookupUser(st.Uid), lookupGroup(st.Gid)
	m.devMaj, m.devMin = unix.Major(uint64(st.Dev)), unix.Minor(uint64(st.Dev))
	m.rdevMaj, m.rdevMin = unix.Major(uint64(st.Rdev)), unix.Minor(uint64(st.Rdev))
	m.ino = st.Ino
	m.nlink = uint64(st.Nlink)
	m.atime = time.Unix(st.Atim.Unix())
	m.mtime = time.Unix(st.Mtim.Unix())
	m.ctime = time.Unix(st.Ctim.Unix())
	// Birth time needs statx(2), which plain stat does not surface;
	// GNU prints "-" when the birth time is unknown.
}
