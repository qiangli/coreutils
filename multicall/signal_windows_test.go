//go:build windows

package multicall

import "testing"

// On Windows TerminateBySignal is an explicit no-op: there is no POSIX signal
// wait-status model to reproduce, so it must always return and let the boundary
// exit normally. (env's commandSignalOutcome never sets RunContext.ExitSignal
// on Windows, so this is not reached in practice either.)
func TestTerminateBySignalIsNoOpOnWindows(t *testing.T) {
	TerminateBySignal(0)
	TerminateBySignal(15)
	TerminateBySignal(-1)
	// Reaching here at all is the assertion: the process was not terminated.
}
