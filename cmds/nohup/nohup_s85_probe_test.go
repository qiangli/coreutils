//go:build unix

package nohupcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"github.com/qiangli/coreutils/tool"
)

// TestS85ProbeExitCodes isolates 0, 126, and 127 exit statuses for nohup.
func TestS85ProbeExitCodes(t *testing.T) {
	dir := t.TempDir()

	// 1. Success case (0)
	var out0, err0 bytes.Buffer
	rc0 := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{Out: &out0, Err: &err0, In: strings.NewReader("")},
	}
	if code := run(rc0, []string{"true"}); code != 0 {
		t.Fatalf("successful command returned %d, want 0", code)
	}

	// 2. Found but non-executable file (126)
	nonExecPath := filepath.Join(dir, "blocked_cmd")
	if err := os.WriteFile(nonExecPath, []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out126, err126 bytes.Buffer
	rc126 := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=."},
		Stdio: tool.Stdio{Out: &out126, Err: &err126, In: strings.NewReader("")},
	}
	if code := run(rc126, []string{"blocked_cmd"}); code != 126 {
		t.Fatalf("non-executable command returned %d, want 126", code)
	}

	// 3. Command not found (127)
	var out127, err127 bytes.Buffer
	rc127 := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/nowhere"},
		Stdio: tool.Stdio{Out: &out127, Err: &err127, In: strings.NewReader("")},
	}
	if code := run(rc127, []string{"nonexistent_cmd_s85"}); code != 127 {
		t.Fatalf("missing command returned %d, want 127", code)
	}

	// 4. Missing operand (127)
	var outMissing, errMissing bytes.Buffer
	rcMissing := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{Out: &outMissing, Err: &errMissing, In: strings.NewReader("")},
	}
	if code := run(rcMissing, nil); code != 127 {
		t.Fatalf("missing operand returned %d, want 127", code)
	}
}

// TestS85ProbeDescriptorAndAppendMode isolates descriptor redirection,
// file creation mode 0600, and append behavior over pre-existing content.
func TestS85ProbeDescriptorAndAppendMode(t *testing.T) {
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	dir := t.TempDir()
	nohupOutPath := filepath.Join(dir, "nohup.out")

	// Seed pre-existing content in nohup.out
	initialContent := "PRE-EXISTING CONTENT\n"
	if err := os.WriteFile(nohupOutPath, []byte(initialContent), 0o600); err != nil {
		t.Fatal(err)
	}

	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: strings.NewReader(""), Out: pts, Err: pts},
	}
	if code := run(rc, []string{"sh", "-c", "printf 'NEW CONTENT\\n'"}); code != 0 {
		t.Fatalf("code=%d", code)
	}

	// Assert pre-existing content is preserved and new content is appended
	data, err := os.ReadFile(nohupOutPath)
	if err != nil {
		t.Fatal(err)
	}
	wantContent := initialContent + "NEW CONTENT\n"
	if string(data) != wantContent {
		t.Fatalf("nohup.out content = %q, want %q", string(data), wantContent)
	}

	// Assert mode is 0600
	fi, err := os.Stat(nohupOutPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("nohup.out permissions = 0%o, want 0600", perm)
	}
}

// TestS85ProbeWriteAndKillHangup isolates SIGHUP signal immunity
// for a long-running process writing to nohup.out.
func TestS85ProbeWriteAndKillHangup(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	nohupOut := filepath.Join(dir, "nohup.out")

	// Create a test script that writes its PID to a file, sleeps, and writes output.
	script := filepath.Join(dir, "write_loop.sh")
	scriptBody := fmt.Sprintf("#!/bin/sh\necho $$ > %q\nsleep 0.2\necho 'S85_PROBE_WRITE_SUCCESS'\n", pidFile)
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}

	ptm, pts, err := pty.Open()
	if err != nil {
		t.Skipf("pty.Open failed: %v", err)
	}
	defer ptm.Close()
	defer pts.Close()

	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   []string{"PATH=/bin:/usr/bin"},
		Stdio: tool.Stdio{In: pts, Out: pts, Err: pts},
	}

	runDone := make(chan int, 1)
	go func() {
		runDone <- run(rc, []string{script})
	}()

	// Read child PID
	var pid int
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if n, scanErr := fmt.Sscanf(string(data), "%d", &pid); err == nil && scanErr == nil && n == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("child process failed to write PID file")
	}

	// Send SIGHUP to the child process
	if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
		t.Fatalf("failed to send SIGHUP to child: %v", err)
	}

	select {
	case code := <-runDone:
		if code != 0 {
			t.Fatalf("command exited with code %d after SIGHUP, want 0", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for command after SIGHUP")
	}

	// Check output was written to nohup.out
	data, err := os.ReadFile(nohupOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "S85_PROBE_WRITE_SUCCESS") {
		t.Fatalf("nohup.out does not contain expected output: %q", string(data))
	}
}
