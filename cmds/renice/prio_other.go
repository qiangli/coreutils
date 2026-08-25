//go:build !unix

package renicecmd

import (
	"fmt"
	"runtime"
)

// Windows scheduling classes (and other non-unix schedulers) do not map
// onto POSIX nice values, so renice fails closed once at invocation —
// after parsing, before any ID is touched — rather than erroring one ID
// at a time or approximating a different scheduler under this name.
const (
	whichProcess = 0
	whichPGroup  = 1
	whichUser    = 2
)

func newHostScheduler() (scheduler, error) {
	return nil, fmt.Errorf("nice values are not supported on %s: POSIX process scheduling priorities do not exist on this platform", runtime.GOOS)
}
