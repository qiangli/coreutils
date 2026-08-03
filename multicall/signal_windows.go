//go:build windows

package multicall

// TerminateBySignal is a no-op on Windows: there is no POSIX signal wait-status
// model to reproduce, and env's commandSignalOutcome never sets
// RunContext.ExitSignal there, so the standalone boundary always exits normally
// with the returned status.
func TerminateBySignal(sig int) {}
