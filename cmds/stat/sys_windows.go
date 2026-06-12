//go:build windows

package statcmd

import (
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// fillSys on Windows: inode / link count / uid / gid / block count
// have no portable equivalent and report 0 / 1 / 0 / 0 / a
// size-derived value. Owner and group names are a best-effort SID
// account lookup ("UNKNOWN" when unavailable). The change time
// reports the last write time; the birth time comes from the file's
// creation time.
func fillSys(m *fileMeta, path string, fi os.FileInfo) {
	m.blocks = (fi.Size() + 511) / 512
	m.ioBlock = 4096
	m.nlink = 1
	m.uname, m.gname = ownerGroup(path)
	if d, ok := fi.Sys().(*syscall.Win32FileAttributeData); ok {
		m.atime = time.Unix(0, d.LastAccessTime.Nanoseconds())
		m.mtime = time.Unix(0, d.LastWriteTime.Nanoseconds())
		m.ctime = m.mtime
		m.birth = time.Unix(0, d.CreationTime.Nanoseconds())
		m.hasBirth = true
	} else {
		m.atime, m.ctime = m.mtime, m.mtime
	}
}

func ownerGroup(path string) (string, string) {
	owner, group := "UNKNOWN", "UNKNOWN"
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION)
	if err != nil {
		return owner, group
	}
	if sid, _, oerr := sd.Owner(); oerr == nil && sid != nil {
		if acct, _, _, lerr := sid.LookupAccount(""); lerr == nil {
			owner = acct
		}
	}
	if sid, _, gerr := sd.Group(); gerr == nil && sid != nil {
		if acct, _, _, lerr := sid.LookupAccount(""); lerr == nil {
			group = acct
		}
	}
	return owner, group
}
