//go:build unix

package teecmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type signalReadyReader struct{ announced bool }

func (r *signalReadyReader) Read(p []byte) (int, error) {
	if !r.announced {
		r.announced = true
		_, _ = os.Stdout.WriteString("ready\n")
	}
	return os.Stdin.Read(p)
}

func TestTeeSignalHelper(t *testing.T) {
	if os.Getenv("GO_WANT_TEE_SIGNAL_HELPER") != "1" {
		return
	}
	args := []string(nil)
	if os.Getenv("GO_TEE_IGNORE_INTERRUPT") == "1" {
		args = []string{"-i"}
	}
	if os.Getenv("GO_TEE_BROKEN_STDOUT") == "1" {
		fmt.Fprintln(os.Stderr, "ready")
		rc := &tool.RunContext{Ctx: context.Background(), Dir: os.TempDir(), Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr}}
		os.Exit(cmd.Run(rc, args))
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   os.TempDir(),
		Stdio: tool.Stdio{In: &signalReadyReader{}, Out: io.Discard, Err: os.Stderr},
	}
	os.Exit(cmd.Run(rc, args))
}

func TestTeeDefaultSIGPIPEDisposition(t *testing.T) {
	child := exec.Command(os.Args[0], "-test.run=^TestTeeSignalHelper$")
	child.Env = append(os.Environ(), "GO_WANT_TEE_SIGNAL_HELPER=1", "GO_TEE_BROKEN_STDOUT=1")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := child.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if ready, err := bufio.NewReader(stderr).ReadString('\n'); err != nil || ready != "ready\n" {
		_ = child.Process.Kill()
		t.Fatalf("helper readiness = (%q, %v)", ready, err)
	}
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, "trigger\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	err = waitSignalHelper(t, child)
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("broken stdout wait error = %v, want SIGPIPE", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGPIPE {
		t.Fatalf("broken stdout status = %v", exitErr.Sys())
	}
}

func startSignalHelper(t *testing.T, ignore bool) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^TestTeeSignalHelper$")
	child.Env = append(os.Environ(), "GO_WANT_TEE_SIGNAL_HELPER=1")
	if ignore {
		child.Env = append(child.Env, "GO_TEE_IGNORE_INTERRUPT=1")
	}
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	if ready, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || ready != "ready\n" {
		_ = child.Process.Kill()
		t.Fatalf("helper readiness = (%q, %v)", ready, err)
	}
	return child, stdin
}

func waitSignalHelper(t *testing.T, child *exec.Cmd) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		<-done
		t.Fatal("tee signal helper did not exit")
		return nil
	}
}

func TestTeeDefaultInterruptDisposition(t *testing.T) {
	child, stdin := startSignalHelper(t, false)
	defer stdin.Close()
	if err := child.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := waitSignalHelper(t, child)
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("default SIGINT wait error = %v, want signal termination", err)
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Fatalf("default SIGINT status = %v", exitErr.Sys())
	}
}

func TestTeeIgnoreInterruptsActual(t *testing.T) {
	child, stdin := startSignalHelper(t, true)
	if err := child.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, "still running\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := waitSignalHelper(t, child); err != nil {
		t.Fatalf("tee -i after SIGINT: %v", err)
	}
}
