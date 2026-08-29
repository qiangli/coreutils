//go:build !windows

package envcmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
)

// A standalone env is an exec utility: COMMAND must retain env's PID. This is
// the process boundary GA42 measures when it signals `env ... COMMAND`; a
// fork-and-wait wrapper can die while COMMAND remains alive and writes a
// spurious broken-pipe diagnostic into the signal-status stream.
func TestEnvDedicatedProcessExecReplacesPID(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	c := exec.Command(exe, "-test.run=^TestEnvHelperProcess$", "--", "dedicated")
	c.Env = append(os.Environ(), helperEnvKey+"=1")
	c.Dir = t.TempDir()
	stdout, err := c.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	c.Stderr = &stderr
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Process.Kill() }()

	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read COMMAND pid: %v; stderr=%q", err, stderr.String())
	}
	gotPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("COMMAND pid %q: %v", line, err)
	}
	if gotPID != c.Process.Pid {
		_ = syscall.Kill(gotPID, syscall.SIGKILL)
		t.Fatalf("COMMAND pid=%d, env pid=%d: dedicated env left a wrapper process", gotPID, c.Process.Pid)
	}
	if err := c.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = c.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("wait err=%v (%T), want signal death; stderr=%q", err, err, stderr.String())
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
		t.Fatalf("wait status=%v, want SIGTERM; stderr=%q", ee.ProcessState.Sys(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("dedicated env/COMMAND wrote stderr after signal: %q", stderr.String())
	}
}

// DedicatedProcess is necessary but not sufficient authority to overlay the
// current process. Virtual cwd semantics or non-process stdio require the
// ordinary fork/wait path so RunContext remains meaningful to the caller.
func TestEnvDedicatedProcessExecRequiresFullProcessContract(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	for _, mode := range []string{"virtual-cwd", "buffered"} {
		t.Run(mode, func(t *testing.T) {
			c := exec.Command(exe, "-test.run=^TestEnvHelperProcess$", "--", "dedicated-guard", mode)
			c.Env = append(os.Environ(), helperEnvKey+"=1")
			out, err := c.CombinedOutput()
			if err != nil {
				t.Fatalf("guard helper: %v; output=%q", err, out)
			}
			if got, want := string(out), "guard-returned=0\n"; got != want {
				t.Fatalf("guard output=%q, want %q (process was unexpectedly replaced)", got, want)
			}
		})
	}
}

func TestEnvDedicatedProcessExecPreservesCommandSpellingAsArgv0(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	c := exec.Command(exe, "-test.run=^TestEnvHelperProcess$", "--", "dedicated-argv0")
	c.Env = append(os.Environ(), helperEnvKey+"=1")
	c.Dir = t.TempDir()
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("argv0 helper: %v; output=%q", err, out)
	}
	if got, want := string(out), "env-command-alias\n"; got != want {
		t.Fatalf("COMMAND argv0=%q, want original spelling %q", got, want)
	}
}

// signalByName resolves a signal name to its number through the same table env
// parses, so the helpers and the assertions agree without a second constant
// list.
func signalByName(t *testing.T, name string) syscall.Signal {
	t.Helper()
	sig, ok := namedSignals[strings.TrimPrefix(strings.ToUpper(name), "SIG")]
	if !ok {
		t.Fatalf("unknown test signal %q", name)
	}
	return sig
}

// helperSelfRaise (child role) terminates this process with the named signal.
// It restores the default disposition first, because the Go runtime and the
// testing framework install handlers for some signals, then re-raises so the
// parent's wait sees a real signal death.
func helperSelfRaise(name string) {
	sig, ok := namedSignals[strings.TrimPrefix(strings.ToUpper(name), "SIG")]
	if !ok {
		fmt.Fprintf(os.Stderr, "helper: unknown signal %q\n", name)
		os.Exit(2)
	}
	multicall.TerminateBySignal(int(sig))
	os.Exit(2) // TerminateBySignal must not return for a terminating signal.
}

// helperStandaloneBoundary (child role) reproduces exactly what the standalone
// multicall binary does: run env with a COMMAND that dies by the named signal,
// then, on a signal-terminated COMMAND, apply the real process boundary. This
// process must end up terminated by the same signal.
func helperStandaloneBoundary(name string) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "helper: %v\n", err)
		os.Exit(2)
	}
	command := []string{exe, "-test.run=^TestEnvHelperProcess$", "--", "raise", name}
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   os.TempDir(),
		Env:   []string{helperEnvKey + "=1"},
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
	}
	code := cmd.Run(rc, command)
	if rc.ExitSignal != 0 {
		multicall.TerminateBySignal(rc.ExitSignal)
	}
	os.Exit(code)
}

// Embedded: env runs a COMMAND killed by a signal in-process. env must report
// the safe 128+N status and the raw signal via RunContext.ExitSignal, and the
// host — this very test process — must SURVIVE. Reaching the assertions at all
// proves env did not signal the host.
func TestEnvExecEmbeddedSignalReportsStatusAndSparesHost(t *testing.T) {
	for _, name := range []string{"TERM", "INT", "USR1", "QUIT", "HUP"} {
		sig := signalByName(t, name)
		args := append(append([]string{}, helperArgv(t)...), "raise", name)

		var out, errb bytes.Buffer
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Dir:   t.TempDir(),
			Env:   []string{helperEnvKey + "=1"},
			FS:    tool.NewLocalFS(),
			Stdio: tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
		}
		code := cmd.Run(rc, args)

		if want := 128 + int(sig); code != want {
			t.Errorf("SIG%s: exit code = %d, want %d (128+N)", name, code, want)
		}
		if rc.ExitSignal != int(sig) {
			t.Errorf("SIG%s: rc.ExitSignal = %d, want %d", name, rc.ExitSignal, int(sig))
		}
		if out.Len() != 0 {
			t.Errorf("SIG%s: env wrote to stdout: %q", name, out.String())
		}

		// Embedders may reuse a RunContext. A later successful COMMAND must
		// clear the boundary metadata rather than inheriting this signal.
		if code := cmd.Run(rc, append(helperArgv(t), "quiet")); code != 0 {
			t.Errorf("SIG%s: reused context normal exit = %d, want 0", name, code)
		}
		if rc.ExitSignal != 0 {
			t.Errorf("SIG%s: reused context retained ExitSignal %d", name, rc.ExitSignal)
		}
	}
}

// Standalone boundary: a dedicated multicall process wrapping a COMMAND killed
// by a signal must itself end up terminated by that exact signal — WIFSIGNALED
// with the same number — matching an execve-replacing GNU env. Includes the
// core-producing SIGQUIT/SIGABRT to prove the default disposition is genuinely
// restored (not merely translated to an exit code).
func TestEnvExecStandaloneBoundaryReRaisesExactSignal(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	for _, name := range []string{"TERM", "INT", "USR1", "QUIT", "ABRT"} {
		t.Run(name, func(t *testing.T) {
			sig := signalByName(t, name)
			c := exec.Command(exe, "-test.run=^TestEnvHelperProcess$", "--", "boundary", name)
			c.Env = append(os.Environ(), helperEnvKey+"=1")
			c.Dir = t.TempDir() // any core file lands here and is cleaned up

			err := c.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("boundary process err = %v (%T), want *exec.ExitError", err, err)
			}
			ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
			if !ok {
				t.Fatalf("no WaitStatus available")
			}
			if !ws.Signaled() {
				t.Fatalf("boundary process exited (code %d), want killed by SIG%s", ee.ExitCode(), name)
			}
			if ws.Signal() != sig {
				t.Errorf("boundary process killed by %v, want %v", ws.Signal(), sig)
			}
		})
	}
}
