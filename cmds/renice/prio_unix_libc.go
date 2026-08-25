//go:build unix && !linux

package renicecmd

import (
	"fmt"
	"runtime"
)

// Darwin, the BSDs, Solaris, and AIX report syscall errors out of band
// (carry flag or libc errno), so getpriority returns the true nice value
// directly — only Linux's raw-syscall encoding needs undoing.
func niceFromGetpriority(raw int) int { return raw }

func (hostScheduler) members(which, id int) ([]int, error) {
	return nil, fmt.Errorf("process-group and user adjustments are not supported on %s: exact per-process membership enumeration is unavailable", runtime.GOOS)
}
