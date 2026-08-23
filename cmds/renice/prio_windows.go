//go:build windows

package renicecmd

import "errors"

// Windows scheduling classes do not map onto POSIX nice values, so renice has
// nothing faithful to do rather than approximate one onto the other.
const (
	whichProcess = 0
	whichPGroup  = 1
	whichUser    = 2
)

var errUnsupported = errors.New("nice values are not a Windows concept")

func getPriority(int, int) (int, error) { return 0, errUnsupported }
func setPriority(int, int, int) error   { return errUnsupported }
