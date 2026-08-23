// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !windows

// The exec half: argv passthrough and exit-status propagation, driven against a
// fake provider script in a temp cache. Unix-only because the fake provider is a
// #! script; the resolve/registration behaviour it sits on is covered
// cross-platform in posixproviders_test.go.
package posixproviderscmd

import (
	"strings"
	"syscall"
	"testing"
)

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
	provision(t, root, "patch", "#!/bin/sh\nkill -TERM $$\n")

	rc, out, errb := newRC(t, root)
	code, _, stderr := run(t, "patch", rc, out, errb)

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
