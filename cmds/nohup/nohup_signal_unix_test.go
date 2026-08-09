//go:build unix

package nohupcmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

func TestNohupEnvironmentHelper(t *testing.T) {
	if os.Getenv("GO_WANT_NOHUP_ENV_HELPER") == "" {
		return
	}
	fmt.Fprint(os.Stdout, strings.Join(os.Environ(), "\n"))
	os.Exit(0)
}

func TestNohupPreservesInvocationEnvironment(t *testing.T) {
	t.Setenv("NOHUP_HOST_LEAK", "host-value")
	var out, errOut bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   t.TempDir(),
		Env:   []string{"GO_WANT_NOHUP_ENV_HELPER=1", "ONLY=invocation-value"},
		Stdio: tool.Stdio{Out: &out, Err: &errOut},
	}
	if code := run(rc, []string{os.Args[0], "-test.run=^TestNohupEnvironmentHelper$"}); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	got := make(map[string]string)
	for _, entry := range strings.Split(out.String(), "\n") {
		name, value, ok := strings.Cut(entry, "=")
		if ok {
			got[name] = value
		}
	}
	if got["GO_WANT_NOHUP_ENV_HELPER"] != "1" || got["ONLY"] != "invocation-value" {
		t.Fatalf("invocation environment not preserved: %q", out.String())
	}
	if _, ok := got["NOHUP_HOST_LEAK"]; ok {
		t.Fatalf("inherited host environment: %q", out.String())
	}
}

func TestNohupSignalHelper(t *testing.T) {
	pidPath := os.Getenv("GO_WANT_NOHUP_SIGNAL_HELPER")
	if pidPath == "" {
		return
	}
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0o600); err != nil {
		os.Exit(2)
	}
	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}

// TestNohupSignalDriverHelper gives the nohup invocation a known default
// SIGHUP disposition without changing the signal state of the test process.
// This prevents a wrapper that happened to ignore SIGHUP from making the
// child test pass accidentally.
func TestNohupSignalDriverHelper(t *testing.T) {
	pidPath := os.Getenv("GO_WANT_NOHUP_SIGNAL_DRIVER")
	if pidPath == "" {
		return
	}
	signal.Reset(syscall.SIGHUP)
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   filepath.Dir(pidPath),
		Env:   append(os.Environ(), "GO_WANT_NOHUP_SIGNAL_HELPER="+pidPath),
		Stdio: tool.Stdio{Out: io.Discard, Err: io.Discard},
	}
	os.Exit(run(rc, []string{os.Args[0], "-test.run=^TestNohupSignalHelper$"}))
}

func TestNohupIgnoresHangupForChild(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "child.pid")
	driver := exec.Command(os.Args[0], "-test.run=^TestNohupSignalDriverHelper$")
	driver.Env = append(os.Environ(), "GO_WANT_NOHUP_SIGNAL_DRIVER="+pidPath)
	result := make(chan error, 1)
	if err := driver.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { result <- driver.Wait() }()

	var pid int
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidPath)
		if n, scanErr := fmt.Sscanf(string(data), "%d", &pid); err == nil && scanErr == nil && n == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 {
		_ = driver.Process.Kill()
		<-result
		t.Fatal("child did not report its pid")
	}
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatalf("nohup child exited after SIGHUP: %v", err)
	}
}
