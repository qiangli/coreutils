// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

// The exec half: argv passthrough and exit-status propagation, driven against a
// fake provider script in a temp cache. Unix-only because the fake provider is a
// #! script; the resolve/registration behaviour it sits on is covered
// cross-platform in posixproviders_test.go.
package posixproviderscmd

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

const dedicatedExecHelperEnv = "BASHY_POSIX_PROVIDER_DEDICATED_EXEC_HELPER"

// TestDedicatedProcessExecPreservesProviderPID is the public reducer for the
// standalone signal boundary.  A passthrough provider must replace the
// multicall process so a signal sent to the utility PID reaches the provider;
// fork-and-wait would leave a second, orphanable process behind.
func TestDedicatedProcessExecPreservesProviderPID(t *testing.T) {
	if path := os.Getenv(dedicatedExecHelperEnv); path != "" {
		rc := &tool.RunContext{
			DedicatedProcess: true,
			DirIsProcessCwd:  true,
			Env:              os.Environ(),
			Stdio:            tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
		}
		attempted, code := execProviderDedicated(rc, "lp", path, []string{"ready"})
		if !attempted {
			os.Exit(125)
		}
		os.Exit(code)
	}

	dir := t.TempDir()
	provider := dir + "/lp-provider"
	if err := os.WriteFile(provider, []byte("#!/bin/sh\nprintf '%s %s\\n' \"$$\" \"$1\"\nkill -STOP $$\nread line\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	child := exec.Command(os.Args[0], "-test.run=^TestDedicatedProcessExecPreservesProviderPID$")
	child.Env = append(os.Environ(), dedicatedExecHelperEnv+"="+provider)
	var stderr bytes.Buffer
	child.Stderr = &stderr
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}

	line := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		if s.Scan() {
			line <- s.Text()
			return
		}
		line <- ""
	}()
	var got string
	select {
	case got = <-line:
	case <-time.After(5 * time.Second):
		_ = child.Process.Kill()
		t.Fatal("provider did not report its process identity")
	}
	providerPID, marker, ok := strings.Cut(got, " ")
	if !ok || marker != "ready" {
		_ = child.Process.Kill()
		t.Fatalf("provider handshake = %q, stderr=%q", got, stderr.String())
	}
	wantPID := strconv.Itoa(child.Process.Pid)
	if providerPID != wantPID {
		_ = child.Process.Kill()
		t.Fatalf("provider PID = %s, advertised utility PID = %s; provider was not exec-replaced", providerPID, wantPID)
	}

	if err := child.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if err := child.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	err = child.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("wait = %v, stderr=%q; want signal termination", err, stderr.String())
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
		t.Fatalf("wait status = %v, stderr=%q; want SIGTERM", exitErr.ProcessState, stderr.String())
	}
}

func TestExitStatusPropagation(t *testing.T) {
	for _, want := range []int{0, 1, 2, 42, 127} {
		root := t.TempDir()
		provision(t, root, "ctags", "#!/bin/sh\nexit "+itoa(want)+"\n")

		rc, out, errb := newRC(t, root)
		code, _, stderr := run(t, "ctags", rc, out, errb)
		if code != want {
			t.Errorf("exit = %d, want %d (stderr %q)", code, want, stderr)
		}
		if rc.ExitSignal != 0 {
			t.Errorf("ExitSignal = %d on a normal exit, want 0", rc.ExitSignal)
		}
	}
}

// TestSignalDeathPropagation: a provider killed by a signal must report the safe
// 128+N to every caller AND record the raw signal, so multicall.Main can
// re-raise it and inherit the exact wait status the way an execve-replacing
// wrapper would.
func TestSignalDeathPropagation(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "ctags", "#!/bin/sh\nkill -TERM $$\n")

	rc, out, errb := newRC(t, root)
	code, _, stderr := run(t, "ctags", rc, out, errb)

	wantSig := int(syscall.SIGTERM)
	if code != 128+wantSig {
		t.Errorf("exit = %d, want %d (stderr %q)", code, 128+wantSig, stderr)
	}
	if rc.ExitSignal != wantSig {
		t.Errorf("ExitSignal = %d, want %d", rc.ExitSignal, wantSig)
	}
}

// TestStdinReachesTheProvider: the provider inherits the invocation's stdio, so
// a filter such as `bc` reads the caller's input, not the process's.
func TestStdinReachesTheProvider(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "m4", "#!/bin/sh\nwhile IFS= read -r l; do printf 'in:%s\\n' \"$l\"; done\n")

	rc, out, errb := newRC(t, root)
	rc.In = strings.NewReader("one\ntwo\n")
	code, stdout, stderr := run(t, "m4", rc, out, errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if stdout != "in:one\nin:two\n" {
		t.Errorf("stdout = %q", stdout)
	}
}

// TestProviderEnvironmentComesFromTheRunContext: the embedding shell owns the
// environment, and a nil rc.Env means an EMPTY one, never the host process's.
func TestProviderEnvironmentComesFromTheRunContext(t *testing.T) {
	root := t.TempDir()
	provision(t, root, "ar", "#!/bin/sh\nprintf 'MARKER=%s\\n' \"${MARKER:-unset}\"\n")

	rc, out, errb := newRC(t, root, "MARKER=from-runcontext")
	code, stdout, stderr := run(t, "ar", rc, out, errb)
	if code != 0 {
		t.Fatalf("exit = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "MARKER=from-runcontext") {
		t.Errorf("stdout = %q, want the RunContext's value", stdout)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
