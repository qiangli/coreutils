//go:build !windows

package multicall

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
)

// When re-executed with this marker set to a signal number, the test binary
// calls the real TerminateBySignal and must die by that signal.
const terminateMarker = "COREUTILS_MULTICALL_TERMINATE_SIGNAL"

func TestMain(m *testing.M) {
	if v := os.Getenv(terminateMarker); v != "" {
		sig, _ := strconv.Atoi(v)
		TerminateBySignal(sig)
		// Only reached if the signal did not terminate us — a failure the
		// parent detects as a non-signal exit.
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TerminateBySignal restores the default disposition and re-raises the signal
// on this process, so a standalone multicall boundary inherits a wrapped
// COMMAND's exact wait status: WIFSIGNALED with the same signal number.
func TestTerminateBySignalReRaisesExactSignal(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	// A spread across terminating signals, including the core-producing
	// SIGQUIT/SIGABRT, to prove the default action is genuinely restored.
	for _, sig := range []syscall.Signal{
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGUSR1, syscall.SIGQUIT, syscall.SIGABRT,
	} {
		t.Run(sig.String(), func(t *testing.T) {
			c := exec.Command(exe, "-test.run=^$")
			c.Env = append(os.Environ(), terminateMarker+"="+strconv.Itoa(int(sig)))
			c.Dir = t.TempDir() // any core file lands here, cleaned up with the dir

			err := c.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("process err = %v (%T), want *exec.ExitError", err, err)
			}
			ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
			if !ok {
				t.Fatalf("no WaitStatus available")
			}
			if !ws.Signaled() {
				t.Fatalf("process exited (code %d), want killed by %v", ee.ExitCode(), sig)
			}
			if ws.Signal() != sig {
				t.Errorf("process killed by %v, want %v", ws.Signal(), sig)
			}
		})
	}
}

// A zero or negative signal is a no-op: TerminateBySignal must return so the
// boundary falls through to a normal exit.
func TestTerminateBySignalIgnoresNonSignal(t *testing.T) {
	TerminateBySignal(0)
	TerminateBySignal(-1)
}
