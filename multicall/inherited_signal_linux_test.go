//go:build linux

package multicall

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
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
	default:
		os.Exit(2)
	}
}

func TestPreserveInheritedIgnoredSignals(t *testing.T) {
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

func TestPreserveDefaultAbortWithoutGoTraceback(t *testing.T) {
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
