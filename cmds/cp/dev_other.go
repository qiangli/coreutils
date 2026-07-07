//go:build !unix

package cpcmd

import "os"

type devID uint64

func fileDev(fi os.FileInfo) (devID, bool) {
	return 0, false
}
