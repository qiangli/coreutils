//go:build unix && !linux && !darwin && !freebsd

package cpcmd

import (
	"fmt"
	"os"
)

func deviceNumber(fi os.FileInfo) (uint64, bool) {
	return 0, false
}

func makeDeviceNode(path string, mode uint32, device uint64) error {
	return fmt.Errorf("device-node recreation unsupported on this platform")
}
