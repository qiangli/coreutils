//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package ddcmd

import (
	"errors"
	"os"
	"runtime"
)

var errUnsupportedInterruptibleNamedFIFOInput = errors.New(
	"interruptible named FIFO input is not supported on " + runtime.GOOS,
)

// A blocking FIFO open cannot be made cancellable safely after unlink or
// rename, and these kernels have not established Linux's observable
// writer-open-close transition. Refuse only this boundary; already-open input
// streams and output FIFO opens/writes retain descriptor-level cancellation.
func interruptibleOpenNamedFIFORead(string, *interruptContext) (*os.File, error) {
	return nil, errUnsupportedInterruptibleNamedFIFOInput
}
