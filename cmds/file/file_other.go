//go:build !unix

package filecmd

import "os"

func specialDeviceType(info os.FileInfo) string {
	if info.Mode()&os.ModeCharDevice != 0 {
		return "character special"
	}
	return "block special"
}
