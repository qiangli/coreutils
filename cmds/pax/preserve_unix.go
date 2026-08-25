//go:build unix

package paxcmd

import "os"

func defaultChownExtracted(path string, uid, gid int, symlink bool) error {
	if symlink {
		return os.Lchown(path, uid, gid)
	}
	return os.Chown(path, uid, gid)
}
