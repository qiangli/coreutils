//go:build unix

package findcmd

// Signal-mapping and POSIX empty-PATH-element coverage for -exec. Unix
// only: signals and the executable bit are POSIX concepts, and the
// cross-OS gate (crossvet) compiles the Windows stub in exec_signal_other_test.go.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// raiseSelfSignal kills the helper process with signal n; it does not
// return for a terminating signal. Blocks afterward so a caught/ignored
// signal never falls through to a clean exit and masks the test intent.
func raiseSelfSignal(n int) {
	syscall.Kill(syscall.Getpid(), syscall.Signal(n))
	select {}
}

// TestRunArgvSignaledExitCode drives runArgv directly: a child killed by
// a signal must report 128+signal, not a flat 128. The ; and + forms
// only expose this through their truth value / find's own exit, so the
// mapping is asserted white-box here.
func TestRunArgvSignaledExitCode(t *testing.T) {
	bin := helperBin(t)
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT} {
		rc := &tool.RunContext{
			Ctx: context.Background(),
			Dir: t.TempDir(),
			Env: helperEnv("FIND_HELPER_QUIET=1", "FIND_HELPER_SIGNAL="+strconv.Itoa(int(sig))),
			Stdio: tool.Stdio{
				In:  strings.NewReader(""),
				Out: io.Discard,
				Err: io.Discard,
			},
		}
		w := &walker{rc: rc, owners: newOwnerCache()}
		code, err := w.runArgv([]string{bin}, false)
		if err != nil {
			t.Fatalf("signal %d: runArgv error: %v", sig, err)
		}
		if want := 128 + int(sig); code != want {
			t.Errorf("signal %d: exit code=%d, want %d (128+signal)", sig, code, want)
		}
	}
}

// TestFindExecEmptyPathElement covers the POSIX PATH rules for -exec's
// utility lookup: a zero-length PATH element (including a wholly empty
// PATH) means the working directory, an unset PATH falls back to the
// default search path (never the working directory), and a name with a
// separator resolves against the working directory without consulting
// PATH.
func TestFindExecEmptyPathElement(t *testing.T) {
	dir := setupTree(t)
	// A named executable living in the working directory (rc.Dir).
	script := filepath.Join(dir, "find-exec-local")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf LOCAL\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Explicitly empty PATH: one zero-length element -> the working
	// directory only. The bare name resolves to ./find-exec-local.
	out, errb, code := runFindExec(t, dir, "", []string{"PATH="},
		".", "-name", "a.txt", "-exec", "find-exec-local", "{}", ";")
	if code != 0 || out != "LOCAL" || errb != "" {
		t.Errorf("empty PATH working-dir lookup = (%q, %q, %d), want (%q, \"\", 0)", out, errb, code, "LOCAL")
	}

	// A trailing empty element inside a non-empty PATH means the working
	// directory too, even when the real entries do not contain the name.
	out, errb, code = runFindExec(t, dir, "", []string{"PATH=/no-such-find-dir:"},
		".", "-name", "a.txt", "-exec", "find-exec-local", "{}", ";")
	if code != 0 || out != "LOCAL" || errb != "" {
		t.Errorf("trailing empty element = (%q, %q, %d)", out, errb, code)
	}

	// Unset PATH must NOT fall back to the working directory: the default
	// search path is /usr/bin:/bin, where find-exec-local does not exist,
	// so the utility is not found (exit 1, diagnostic on stderr).
	out, errb, code = runFindExec(t, dir, "", nil,
		".", "-name", "a.txt", "-exec", "find-exec-local", "{}", ";")
	if code != 1 || out != "" || !strings.Contains(errb, "find-exec-local") {
		t.Errorf("unset PATH lookup = (%q, %q, %d), want not-found (exit 1)", out, errb, code)
	}

	// A name with a separator resolves against the working directory,
	// ignoring PATH entirely.
	out, errb, code = runFindExec(t, dir, "", []string{"PATH=/no-such-find-dir"},
		".", "-name", "a.txt", "-exec", "./find-exec-local", "{}", ";")
	if code != 0 || out != "LOCAL" || errb != "" {
		t.Errorf("explicit relative path = (%q, %q, %d)", out, errb, code)
	}
}
