//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/multicall"
	"golang.org/x/sys/unix"
)

const coreutilsSignalHelper = "COREUTILS_MAIN_SIGNAL_HELPER"
const coreutilsDDSignalHelper = "COREUTILS_MAIN_DD_SIGNAL_HELPER"
const coreutilsDDSignalArgs = "COREUTILS_MAIN_DD_SIGNAL_ARGS"

// TestCoreutilsEnvStandaloneSignalBoundary is both the parent assertion and
// the subprocess helper. The boundary role calls the real cmd/coreutils main,
// not a facsimile, so this pins the actual standalone multicall behavior.
func TestCoreutilsEnvStandaloneSignalBoundary(t *testing.T) {
	if os.Getenv(coreutilsSignalHelper) == "1" {
		args := argsAfterDoubleDash(os.Args)
		if len(args) != 2 {
			os.Exit(2)
		}
		sig := signalNumber(args[1])
		switch args[0] {
		case "raise":
			multicall.TerminateBySignal(int(sig))
			os.Exit(2)
		case "boundary":
			exe, err := os.Executable()
			if err != nil {
				os.Exit(2)
			}
			os.Args = []string{
				"coreutils", "env", exe,
				"-test.run=^TestCoreutilsEnvStandaloneSignalBoundary$",
				"--", "raise", args[1],
			}
			main()
			os.Exit(2)
		default:
			os.Exit(2)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	for _, name := range []string{"TERM", "INT", "USR1", "QUIT", "ABRT"} {
		t.Run(name, func(t *testing.T) {
			sig := signalNumber(name)
			cmd := exec.Command(exe,
				"-test.run=^TestCoreutilsEnvStandaloneSignalBoundary$",
				"--", "boundary", name)
			cmd.Env = append(os.Environ(), coreutilsSignalHelper+"=1")
			cmd.Dir = t.TempDir()

			err := cmd.Run()
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatalf("coreutils env boundary err = %v (%T), want signal exit", err, err)
			}
			ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
			if !ok || !ws.Signaled() {
				t.Fatalf("coreutils env exited normally (%d), want killed by SIG%s", ee.ExitCode(), name)
			}
			if ws.Signal() != sig {
				t.Errorf("coreutils env killed by %v, want %v", ws.Signal(), sig)
			}
		})
	}
}

// TestCoreutilsDdStandaloneSIGINTBoundary drives the real multicall entrypoint
// so the assertion covers actual process wait status, not a facsimile of it.
// Both halves of dd's cancellable I/O are exercised: parked in a read from a
// FIFO nobody writes to, and parked in a write to a FIFO nobody drains.
func TestCoreutilsDdStandaloneSIGINTBoundary(t *testing.T) {
	if os.Getenv(coreutilsDDSignalHelper) == "1" {
		var args []string
		if err := json.Unmarshal([]byte(os.Getenv(coreutilsDDSignalArgs)), &args); err != nil {
			os.Exit(2)
		}
		os.Args = append([]string{"coreutils", "dd"}, args...)
		main()
		os.Exit(2)
	}

	exe, err := os.Executable()
	if err != nil {
		t.Skipf("os.Executable unavailable: %v", err)
	}
	t.Run("BlockedRead", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("exact interruptible named-FIFO input is currently supported only on Linux")
		}
		dir := t.TempDir()
		fifo := filepath.Join(dir, "fifo")
		if err := unix.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		cmd, stderr := startDdBoundary(t, exe, dir, []string{"if=fifo", "status=noxfer"})
		// The blocking open of the write side completes exactly when dd has
		// finished its own read-side open, so dd is provably parked in read(2).
		w := attachFIFOWriter(t, fifo)
		defer w.Close()
		assertSignaledBySIGINT(t, cmd, stderr)
		if got, want := stderr.String(), "0+0 records in\n0+0 records out\n"; got != want {
			t.Fatalf("stderr=%q want %q", got, want)
		}
	})
	t.Run("BlockedWrite", func(t *testing.T) {
		dir := t.TempDir()
		fifo := filepath.Join(dir, "fifo")
		if err := unix.Mkfifo(fifo, 0o600); err != nil {
			t.Fatal(err)
		}
		// A reader that opens and never reads lets dd's own open succeed and
		// then lets the FIFO buffer fill.
		fd, err := unix.Open(fifo, unix.O_RDONLY|unix.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer unix.Close(fd)
		cmd, stderr := startDdBoundary(t, exe, dir,
			[]string{"if=/dev/zero", "of=fifo", "bs=65536", "count=64", "status=noxfer"})
		time.Sleep(250 * time.Millisecond)
		assertSignaledBySIGINT(t, cmd, stderr)
		if got := strings.Count(stderr.String(), "records in\n"); got != 1 {
			t.Fatalf("records-in blocks=%d stderr=%q", got, stderr.String())
		}
	})
}

func startDdBoundary(t *testing.T, exe, dir string, args []string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(exe, "-test.run=^TestCoreutilsDdStandaloneSIGINTBoundary$")
	cmd.Env = append(os.Environ(),
		coreutilsDDSignalHelper+"=1",
		coreutilsDDSignalArgs+"="+string(encoded),
	)
	cmd.Dir = dir
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd, &stderr
}

func assertSignaledBySIGINT(t *testing.T, cmd *exec.Cmd, stderr *bytes.Buffer) {
	t.Helper()
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("coreutils dd err = %v (%T), want signal exit; stderr=%q", err, err, stderr.String())
	}
	ws, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGINT {
		t.Fatalf("wait status=%v, want SIGINT; stderr=%q", ee.ProcessState.Sys(), stderr.String())
	}
}

// attachFIFOWriter opens the write side of a FIFO; the blocking open returns
// exactly when the reader side has been opened.
func attachFIFOWriter(t *testing.T, path string) *os.File {
	t.Helper()
	type result struct {
		f   *os.File
		err error
	}
	ch := make(chan result, 1)
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		ch <- result{f, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		return r.f
	case <-time.After(5 * time.Second):
		t.Fatal("dd did not open the FIFO for reading before the deadline")
		return nil
	}
}

func argsAfterDoubleDash(args []string) []string {
	for i, arg := range args {
		if arg == "--" {
			return args[i+1:]
		}
	}
	return nil
}

func signalNumber(name string) syscall.Signal {
	switch strings.TrimPrefix(strings.ToUpper(name), "SIG") {
	case "TERM":
		return syscall.SIGTERM
	case "INT":
		return syscall.SIGINT
	case "USR1":
		return syscall.SIGUSR1
	case "QUIT":
		return syscall.SIGQUIT
	case "ABRT":
		return syscall.SIGABRT
	default:
		return 0
	}
}
