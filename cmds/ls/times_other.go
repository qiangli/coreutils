//go:build unix && !darwin && !linux

package lscmd

import (
	"fmt"
	"os"
	"time"
)

// sysTime fallback: the non-mtime timestamp fields are not wired up on
// this platform, so --time/-c/-u fail loudly per the contract (a clear
// error, never a silent mtime approximation).
func sysTime(_ os.FileInfo, _ string, _ timeSel) (time.Time, error) {
	return time.Time{}, fmt.Errorf("only the modification time is supported on this platform")
}
