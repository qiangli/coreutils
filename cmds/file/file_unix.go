//go:build unix

package filecmd

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func specialDeviceType(info os.FileInfo) string {
	kind := "block special"
	if info.Mode()&os.ModeCharDevice != 0 {
		kind = "character special"
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return kind
	}
	device := uint64(stat.Rdev)
	return fmt.Sprintf("%s (%d/%d)", kind, unix.Major(device), unix.Minor(device))
}
