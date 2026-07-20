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

// exitStatus maps the finished process's wait status to the value `time` should
// exit with. A command killed by a signal has no ordinary exit code, so — like
// the shell and GNU/BSD time — report 128+signum rather than a flat sentinel.
func exitStatus(ps *os.ProcessState) int {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok {
		if ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return ws.ExitStatus()
	}
	if code := ps.ExitCode(); code >= 0 {
		return code
	}
	return 128
}
