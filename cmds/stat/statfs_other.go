//go:build !linux && !darwin

package statcmd

import (
	"errors"
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
	return nil, errors.New("not supported on this platform")
}
