//go:build !windows

package multicall

import (
	"runtime"
	"syscall"
)

// TerminateBySignal is the standalone process boundary's response to a command
// wrapper (env) whose COMMAND was killed by a signal, reported through
// RunContext.ExitSignal. It restores that signal's default disposition and
// re-raises it on this process, so a dedicated multicall binary (the VSC SUT
// runs `env` as exactly that) ends up with COMMAND's own wait status —
// WIFSIGNALED with the same signal, and a core dump for the core-producing
// signals — matching an execve-replacing GNU env.
//
// This is deliberately confined to the standalone boundary (multicall.Main and
// callers that own their process). Embedded hosts run tools through Dispatch or
// tool.Tool.Run and must never call this: signalling would kill the host.
//
// For a signal whose default action terminates the process, kill(2) to self
// delivers before returning (POSIX), so this does not return. It only returns
// when the signal cannot terminate the process (blocked, or a non-terminating
// default), leaving the caller to fall back to a normal exit.
func TerminateBySignal(sig int) {
	if sig <= 0 {
		return
	}
	s := syscall.Signal(sig)
	// Go's os/signal.Reset is not sufficient here: the runtime owns handlers
	// for signals such as SIGQUIT, and a signal may remain blocked on the
	// calling thread. Restore SIG_DFL with sigaction and explicitly unblock it
	// before re-raising. Both operations are confined to this process-owning
	// boundary; embedded hosts never call this function.
	if err := restoreDefaultAndUnblock(s); err != nil {
		return
	}
	if err := syscall.Kill(syscall.Getpid(), s); err != nil {
		return
	}
	// Delivery is normally complete before kill returns, but do not race a
	// following os.Exit on kernels that defer it until the thread is scheduled.
	for {
		runtime.Gosched()
	}
}
