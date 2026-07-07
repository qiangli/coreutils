//go:build linux

package statcmd

import (
	"golang.org/x/sys/unix"
)

type fsStat struct {
	blockSize   int64
	totalBlocks int64
	freeBlocks  int64
	availBlocks int64
	totalInodes int64
	freeInodes  int64
	nameMax     int64
	fsType      string
}

func statfsFile(path string) (*fsStat, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return nil, err
	}
	return &fsStat{
		blockSize:   int64(st.Bsize),
		totalBlocks: int64(st.Blocks),
		freeBlocks:  int64(st.Bfree),
		availBlocks: int64(st.Bavail),
		totalInodes: int64(st.Files),
		freeInodes:  int64(st.Ffree),
		nameMax:     int64(st.Namelen),
		fsType:      "unix",
	}, nil
}
