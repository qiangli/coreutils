//go:build unix

package sleepcmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

const sleepSignalHelperEnv = "BASHY_COREUTILS_SLEEP_SIGNAL_HELPER"

// TestSleepSignalHelper is a subprocess entrypoint. The parent invokes this
// test by its real -test.run flag; passing "sleep" as an ordinary argument to
// the test binary would not exercise multicall dispatch and would be a false
// process-level test. A readiness line removes startup/signal timing races.
func TestSleepSignalHelper(t *testing.T) {
	duration := os.Getenv(sleepSignalHelperEnv)
	if duration == "" {
		return
	}
	if _, err := fmt.Fprintln(os.Stdout, "ready"); err != nil {
		os.Exit(125)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Stdio: tool.Stdio{
			In:  strings.NewReader(""),
			Out: io.Discard,
			Err: os.Stderr,
		},
	}
	os.Exit(cmd.Run(rc, []string{duration}))
}

type sleepSignalProcess struct {
	cmd    *exec.Cmd
	wait   <-chan error
	stderr *bytes.Buffer
}

func startSleepSignalProcess(t *testing.T, duration string) *sleepSignalProcess {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestSleepSignalHelper$")
	child.Env = append(os.Environ(), sleepSignalHelperEnv+"="+duration)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- child.Wait() }()
	ready := make(chan error, 1)
	go func() {
		line, readErr := bufio.NewReader(stdout).ReadString('\n')
		if readErr == nil && line != "ready\n" {
			readErr = fmt.Errorf("unexpected readiness line %q", line)
		}
		ready <- readErr
	}()

	select {
	case err := <-ready:
		if err != nil {
			_ = child.Process.Kill()
			<-wait
			t.Fatalf("signal helper readiness: %v; stderr=%q", err, stderr.String())
		}
	case err := <-wait:
		t.Fatalf("signal helper exited before readiness: %v; stderr=%q", err, stderr.String())
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		<-wait
		t.Fatalf("signal helper readiness timed out; stderr=%q", stderr.String())
	}
	return &sleepSignalProcess{cmd: child, wait: wait, stderr: &stderr}
}

func waitSleepSignalProcess(t *testing.T, p *sleepSignalProcess) error {
	t.Helper()
	select {
	case err := <-p.wait:
		return err
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		err := <-p.wait
		t.Fatalf("signal helper did not terminate within bound: %v; stderr=%q", err, p.stderr.String())
		return nil
	}
}

// TestSleepSIGALRMPermittedDisposition accepts exactly the three Issue 7
// choices. With the helper's one-second operand, effective ignore completes
// the full requested sleep inside the five-second watchdog; default action is
// observed as SIGALRM, and a caught normal completion must have status zero.
func TestSleepSIGALRMPermittedDisposition(t *testing.T) {
	p := startSleepSignalProcess(t, "1")
	if err := syscall.Kill(p.cmd.Process.Pid, syscall.SIGALRM); err != nil {
		_ = p.cmd.Process.Kill()
		<-p.wait
		t.Fatal(err)
	}
	err := waitSleepSignalProcess(t, p)
	if err == nil {
		return // normal zero or effective ignore followed by full-duration zero
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("SIGALRM wait error=%v; stderr=%q", err, p.stderr.String())
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGALRM {
		t.Fatalf("SIGALRM disposition=%v; stderr=%q", exitErr.ProcessState, p.stderr.String())
	}
}

// TestSleepSIGTERMStandardAction proves a non-SIGALRM signal retains its
// standard terminating disposition at a real process boundary.
func TestSleepSIGTERMStandardAction(t *testing.T) {
	p := startSleepSignalProcess(t, "30")
	if err := syscall.Kill(p.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		_ = p.cmd.Process.Kill()
		<-p.wait
		t.Fatal(err)
	}
	err := waitSleepSignalProcess(t, p)
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("SIGTERM returned %v, want signal termination; stderr=%q", err, p.stderr.String())
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("SIGTERM disposition=%v; stderr=%q", exitErr.ProcessState, p.stderr.String())
	}
}
