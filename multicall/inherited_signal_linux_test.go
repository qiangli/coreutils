//go:build linux

package multicall

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "github.com/qiangli/coreutils/cmds/expr"
	_ "github.com/qiangli/coreutils/cmds/tr"
)

func runInheritedSignalHelper(mode string) {
	preserveInheritedSignalDispositions()
	switch mode {
	case "term":
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		os.Exit(0)
	case "abrt":
		_ = syscall.Kill(os.Getpid(), syscall.SIGABRT)
		os.Exit(0)
	case "pipe":
		r, w, err := os.Pipe()
		if err != nil {
			os.Exit(2)
		}
		_ = r.Close()
		_ = syscall.Close(1)
		fd, err := syscall.Dup(int(w.Fd()))
		if err != nil || fd != 1 {
			os.Exit(2)
		}
		_ = w.Close()
		_, err = os.Stdout.Write([]byte("x"))
		if !errors.Is(err, syscall.EPIPE) {
			os.Exit(3)
		}
		os.Exit(0)
	case "pipe_default":
		if processRunContext().SIGPIPEIgnored {
			os.Exit(10)
		}
		os.Exit(0)
	case "pipe_rc", "pipe_expr_rc":
		// Full boundary-seam test: inherited ignored SIGPIPE must reach
		// processRunContext().SIGPIPEIgnored, then flow into each tool's
		// deterministic EPIPE path. preserveInheritedSignalDispositions
		// has already run at the top of this helper.
		rc := processRunContext()
		if !rc.SIGPIPEIgnored {
			os.Stderr.WriteString("pipe_rc: SIGPIPEIgnored=false, want true\n")
			os.Exit(10)
		}
		// Redirect stdout to a write-only pipe with no reader (EPIPE on write).
		r, w, err := os.Pipe()
		if err != nil {
			os.Exit(2)
		}
		_ = r.Close()
		_ = syscall.Close(1)
		fd, err := syscall.Dup(int(w.Fd()))
		if err != nil || fd != 1 {
			os.Exit(2)
		}
		_ = w.Close()
		// Rebuild RunContext so rc.Out is the now-broken fd 1.
		rc = processRunContext()
		// Feed tr a few bytes so its flush path hits EPIPE.
		pr, pw, err := os.Pipe()
		if err != nil {
			os.Exit(2)
		}
		rc.Stdio.In = pr
		_, _ = pw.Write([]byte("abc\n"))
		_ = pw.Close()
		name, args := "tr", []string{"a-z", "A-Z"}
		if mode == "pipe_expr_rc" {
			name, args = "expr", []string{strings.Repeat("x", 8192)}
		}
		code := Dispatch(rc, name, args)
		os.Exit(code)
	case "ttin":
		// Put the child in a non-orphaned process group so POSIX permits the
		// terminal-input signal's default stop action.
		if err := syscall.Setpgid(0, 0); err != nil {
			os.Exit(2)
		}
		_ = syscall.Kill(os.Getpid(), syscall.SIGTTIN)
		os.Exit(0)
	case "ttin_ignored":
		if err := syscall.Setpgid(0, 0); err != nil {
			os.Exit(2)
		}
		_ = syscall.Kill(os.Getpid(), syscall.SIGTTIN)
		os.Exit(0)
	default:
		os.Exit(2)
	}
}

func TestPreserveDefaultTerminalStop(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), inheritedSignalMarker+"=ttin")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	defer func() {
		if pid <= 0 {
			return
		}
		_ = syscall.Kill(pid, syscall.SIGCONT)
		_ = syscall.Kill(pid, syscall.SIGKILL)
		var ws syscall.WaitStatus
		_, _ = syscall.Wait4(pid, &ws, 0, nil)
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var ws syscall.WaitStatus
		got, err := syscall.Wait4(pid, &ws, syscall.WNOHANG|syscall.WUNTRACED, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got == pid {
			if !ws.Stopped() || ws.StopSignal() != syscall.SIGTTIN {
				t.Fatalf("child status = %v, want stopped by SIGTTIN", ws)
			}
			if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
				t.Fatal(err)
			}
			var exit syscall.WaitStatus
			if got, err := syscall.Wait4(pid, &exit, 0, nil); err != nil || got != pid {
				t.Fatalf("wait after SIGCONT: pid=%d err=%v", got, err)
			}
			pid = -1
			if !exit.Exited() || exit.ExitStatus() != 0 {
				t.Fatalf("child after SIGCONT = %v, want clean exit", exit)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not stop for default SIGTTIN")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestPreserveInheritedIgnoredSignals(t *testing.T) {
	if !inheritedSignalSnapshotAvailable() {
		t.Skip("test binary does not expose the runtime signal snapshot")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode string
		sig  string
	}{
		{mode: "term", sig: "TERM"},
		{mode: "abrt", sig: "ABRT"},
		{mode: "pipe", sig: "PIPE"},
		{mode: "ttin_ignored", sig: "TTIN"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			script := "trap '' " + tc.sig + "; exec \"$1\" -test.run=^$"
			cmd := exec.Command("/bin/sh", "-c", script, "sh", exe)
			cmd.Env = append(os.Environ(), inheritedSignalMarker+"="+tc.mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ignored SIG%s helper: %v; output=%q", tc.sig, err, out)
			}
		})
	}
}

// TestStandaloneSIGPIPEIgnoredReachesRunContext proves the boundary seam:
// when the parent process ignores SIGPIPE (via the shell trap builtin),
// that fact flows through preserveInheritedSignalDispositions into
// processRunContext().SIGPIPEIgnored=true, and the tr and expr EPIPE paths
// emit exact broken-pipe diagnostics with exit code 1, the behavior the GNU
// tools exhibit when SIGPIPE is ignored. Derived from
// TestPreserveInheritedIgnoredSignals.
func TestStandaloneSIGPIPEIgnoredReachesRunContext(t *testing.T) {
	if !inheritedSignalSnapshotAvailable() {
		t.Skip("test binary does not expose the runtime signal snapshot")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// Parent ignores SIGPIPE, then execs the test binary with the pipe_rc marker.
	script := "trap '' PIPE; exec \"$1\" -test.run=^$"
	for _, tc := range []struct{ mode, name string }{{"pipe_rc", "tr"}, {"pipe_expr_rc", "expr"}} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", script, "sh", exe)
			cmd.Env = append(os.Environ(), inheritedSignalMarker+"="+tc.mode)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok || ee.ExitCode() != 1 {
				t.Fatalf("exit error = %v, want code 1; stderr=%q", err, stderr.String())
			}
			want := tc.name + ": stdout: Broken pipe\n"
			if stderr.String() != want {
				t.Fatalf("stderr = %q, want %q", stderr.String(), want)
			}
		})
	}
}

func TestStandaloneSIGPIPEDefaultReachesRunContext(t *testing.T) {
	if !inheritedSignalSnapshotAvailable() {
		t.Skip("test binary does not expose the runtime signal snapshot")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "-c", "trap - PIPE; exec \"$1\" -test.run=^$", "sh", exe)
	cmd.Env = append(os.Environ(), inheritedSignalMarker+"=pipe_default")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("default SIGPIPE helper: %v; output=%q", err, out)
	}
}

func TestPreserveDefaultAbortWithoutGoTraceback(t *testing.T) {
	if !inheritedSignalSnapshotAvailable() {
		t.Skip("test binary does not expose the runtime signal snapshot")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), inheritedSignalMarker+"=abrt")
	cmd.Dir = t.TempDir()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("default SIGABRT err = %v (%T), want signal exit", err, err)
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGABRT {
		t.Fatalf("default SIGABRT status = %v, want SIGABRT", ee.ProcessState)
	}
	if stderr.Len() != 0 {
		t.Fatalf("default SIGABRT emitted Go traceback: %q", stderr.Bytes())
	}
}
