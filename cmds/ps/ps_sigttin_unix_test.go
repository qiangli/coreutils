//go:build unix

package pscmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

const psSIGTTINHelperEnv = "BASHY_PS_SIGTTIN_HELPER"

func TestPSUsesLiveOutputTerminalWidthWhenColumnsUnset(t *testing.T) {
	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable: %v", err)
	}
	defer master.Close()
	defer slave.Close()
	if err := pty.Setsize(slave, &pty.Winsize{Cols: 37, Rows: 12}); err != nil {
		t.Skipf("set pty size: %v", err)
	}
	rc := &tool.RunContext{Env: []string{"LC_ALL=C"}, Stdio: tool.Stdio{Out: slave}}
	if got := displayColumns(rc); got != 37 {
		t.Fatalf("live terminal columns=%d, want 37", got)
	}
	rc.Env = []string{"LC_ALL=C", "COLUMNS=19"}
	if got := displayColumns(rc); got != 19 {
		t.Fatalf("COLUMNS override=%d, want 19", got)
	}
}

// firstWriteGate tells the parent that ps reached its first output write and
// then blocks until the parent continues the process. This makes the signal
// disposition check deterministic instead of racing a short ps invocation.
type firstWriteGate struct {
	ready *os.File
	gate  *os.File
	out   *os.File
	done  bool
}

func (w *firstWriteGate) Write(p []byte) (int, error) {
	if !w.done {
		w.done = true
		if _, err := w.ready.Write([]byte{1}); err != nil {
			return 0, err
		}
		var release [1]byte
		if _, err := w.gate.Read(release[:]); err != nil {
			return 0, err
		}
	}
	return w.out.Write(p)
}

// TestPSRetainsDefaultSIGTTINDisposition pins the product half of the native
// harness's SigWait -w/SIGTTIN preflight. A helper reaches ps's first output
// write, the parent delivers SIGTTIN, and wait4 must observe a real job-control
// stop before SIGCONT allows the same invocation to finish successfully.
func TestPSRetainsDefaultSIGTTINDisposition(t *testing.T) {
	switch os.Getenv(psSIGTTINHelperEnv) {
	case "launcher":
		// The agent/test runner may itself inherit SIGTTIN as ignored. The
		// native SigWait topology supplies the required default disposition.
		// Establish it, then exec a fresh Go runtime so the runner below proves
		// that program initialization and ps preserve rather than replace it.
		signal.Reset(syscall.SIGTTIN)
		env := append([]string(nil), os.Environ()...)
		for i := range env {
			if strings.HasPrefix(env[i], psSIGTTINHelperEnv+"=") {
				env[i] = psSIGTTINHelperEnv + "=runner"
			}
		}
		if err := syscall.Exec(os.Args[0], os.Args, env); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(91)
		}
	case "runner":
		ready := os.NewFile(3, "ps-sigttin-ready")
		gate := os.NewFile(4, "ps-sigttin-gate")
		if ready == nil || gate == nil {
			os.Exit(90)
		}
		defer ready.Close()
		defer gate.Close()
		writer := &firstWriteGate{ready: ready, gate: gate, out: os.Stdout}
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			Env:   []string{"LC_ALL=C", "POSIXLY_CORRECT=1"},
			Stdio: tool.Stdio{Out: writer, Err: os.Stderr},
		}
		p := process{pid: 7, command: "probe", args: "probe", argvKnown: true}
		os.Exit(runWithSource(rc, []string{"-A", "-o", "pid"}, fakeProcessSource{ps: []process{p}}, time.Now))
	}

	readyR, readyW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	gateR, gateW, err := os.Pipe()
	if err != nil {
		readyR.Close()
		readyW.Close()
		t.Fatal(err)
	}
	defer readyR.Close()
	defer gateW.Close()

	out, err := os.CreateTemp(t.TempDir(), "ps-sigttin-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	stderrFile, err := os.CreateTemp(t.TempDir(), "ps-sigttin-stderr-")
	if err != nil {
		t.Fatal(err)
	}
	defer stderrFile.Close()
	stderrText := func() string {
		_ = stderrFile.Sync()
		data, _ := os.ReadFile(stderrFile.Name())
		return string(data)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestPSRetainsDefaultSIGTTINDisposition$")
	cmd.Env = append(os.Environ(), psSIGTTINHelperEnv+"=launcher")
	cmd.ExtraFiles = []*os.File{readyW, gateR}
	cmd.Stdout = out
	cmd.Stderr = stderrFile
	// A job-control stop is discarded for an orphaned process group. Put the
	// helper in its own group while this parent remains in the same session,
	// matching the harness's non-orphan SigWait adapter topology.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	readyW.Close()
	gateR.Close()

	cleanup := func() {
		_ = syscall.Kill(cmd.Process.Pid, syscall.SIGKILL)
		var status syscall.WaitStatus
		_, _ = syscall.Wait4(cmd.Process.Pid, &status, 0, nil)
		_ = cmd.Process.Release()
	}
	finished := false
	defer func() {
		if !finished {
			cleanup()
		}
	}()

	ready := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := readyR.Read(b[:])
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("wait for first ps write: %v (stderr %q)", err, stderrText())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("ps did not reach its first output write (stderr %q)", stderrText())
	}

	if err := syscall.Kill(cmd.Process.Pid, syscall.SIGTTIN); err != nil {
		t.Fatal(err)
	}
	var stopped syscall.WaitStatus
	deadline := time.Now().Add(5 * time.Second)
	for {
		pid, err := syscall.Wait4(cmd.Process.Pid, &stopped, syscall.WUNTRACED|syscall.WNOHANG, nil)
		if err != nil {
			t.Fatalf("wait for SIGTTIN stop: %v", err)
		}
		if pid == cmd.Process.Pid {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("ps did not stop after SIGTTIN")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped.Stopped() || stopped.StopSignal() != syscall.SIGTTIN {
		t.Fatalf("wait status=%v, want stopped by SIGTTIN", stopped)
	}

	if err := syscall.Kill(cmd.Process.Pid, syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if _, err := gateW.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	var terminal syscall.WaitStatus
	if _, err := syscall.Wait4(cmd.Process.Pid, &terminal, 0, nil); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Process.Release()
	finished = true
	if !terminal.Exited() || terminal.ExitStatus() != 0 {
		t.Fatalf("terminal wait status=%v stderr=%q", terminal, stderrText())
	}
	if err := out.Sync(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "PID") || !strings.Contains(got, "7") {
		t.Fatalf("continued ps output=%q", got)
	}
	if got := stderrText(); got != "" {
		t.Fatalf("stderr=%q", got)
	}
}
