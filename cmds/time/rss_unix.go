//go:build unix

package timecmd

import (
	"os"
	"runtime"
	"syscall"
)

// maxRSSKB returns peak resident set size in kilobytes from the process's
// rusage. Linux reports ru_maxrss in KB; the BSDs/macOS report it in bytes, so
// normalize. ok=false when rusage is unavailable.
func maxRSSKB(ps *os.ProcessState) (int64, bool) {
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok || ru == nil {
		return 0, false
	}
	kb := int64(ru.Maxrss)
	if runtime.GOOS == "darwin" || runtime.GOOS == "ios" {
		kb /= 1024 // bytes → KB
	}
	return kb, true
}
