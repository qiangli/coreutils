//go:build windows

package lscmd

import (
	"os"

	"golang.org/x/sys/windows"
)

// sysOf on Windows: owner/group are a best-effort SID account-name
// lookup (blank when the security descriptor or the account name is
// unavailable). Link counts and block counts have no cheap portable
// equivalent: nlink reports 1 and blocks derive from the apparent
// size.
func sysOf(fi os.FileInfo, path string) sysInfo {
	owner, group, ownerNum, groupNum := ownerGroup(path)
	return sysInfo{
		nlink:     1,
		owner:     owner,
		group:     group,
		ownerNum:  ownerNum,
		groupNum:  groupNum,
		blocks512: uint64((fi.Size() + 511) / 512),
	}
}

// inodeOf: Windows file IDs require opening a handle per file; -i
// reports 0 instead (documented fallback).
func inodeOf(_ os.FileInfo) uint64 { return 0 }

// ownerGroup returns the account-name and numeric-ID forms of the
// owner and group. Windows has no POSIX uid/gid, so the "numeric" form
// -n asks for is the SID string, the closest Windows analog (blank
// when the security descriptor or SID is unavailable, same fallback as
// the name form).
func ownerGroup(path string) (owner, group, ownerNum, groupNum string) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION)
	if err != nil {
		return "", "", "", ""
	}
	if sid, _, oerr := sd.Owner(); oerr == nil && sid != nil {
		ownerNum = sid.String()
		if acct, _, _, lerr := sid.LookupAccount(""); lerr == nil {
			owner = acct
		}
	}
	if sid, _, gerr := sd.Group(); gerr == nil && sid != nil {
		groupNum = sid.String()
		if acct, _, _, lerr := sid.LookupAccount(""); lerr == nil {
			group = acct
		}
	}
	return owner, group, ownerNum, groupNum
}
