//go:build windows

package findcmd

import "io/fs"

// Windows has no unix uid/gid/nlink/device identity on FileInfo;
// the parser rejects the predicates that need them with a clear
// unsupported error before evaluation can reach these stubs.
const (
	haveSysStat = false
	haveDev     = false
)

func fileUID(fs.FileInfo) (uint32, bool)  { return 0, false }
func fileGID(fs.FileInfo) (uint32, bool)  { return 0, false }
func fileNlink(fs.FileInfo) (uint64, bool) { return 0, false }
func fileDev(fs.FileInfo) (uint64, bool)  { return 0, false }
