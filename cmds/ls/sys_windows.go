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
	owner, group := ownerGroup(path)
	return sysInfo{
		nlink:     1,
		owner:     owner,
		group:     group,
		blocks512: uint64((fi.Size() + 511) / 512),
	}
}

// inodeOf: Windows file IDs require opening a handle per file; -i
// reports 0 instead (documented fallback).
func inodeOf(_ os.FileInfo) uint64 { return 0 }

func ownerGroup(path string) (string, string) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.GROUP_SECURITY_INFORMATION)
	if err != nil {
		return "", ""
	}
	var owner, group string
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
