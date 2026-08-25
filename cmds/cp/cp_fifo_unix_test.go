//go:build unix

package cpcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

const (
	cpHelperEnv  = "COREUTILS_CP_FIFO_HELPER"
	cpArgsEnv    = "COREUTILS_CP_FIFO_ARGS"
	cpDirEnv     = "COREUTILS_CP_FIFO_DIR"
	cpTestLimit  = 5 * time.Second
	cpPollPeriod = 10 * time.Millisecond
)

// TestCpFIFOHelperProcess runs cp in a disposable subprocess so a regression
// that blocks opening a FIFO cannot hang the package test process.
func TestCpFIFOHelperProcess(t *testing.T) {
	if os.Getenv(cpHelperEnv) != "1" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv(cpArgsEnv)), &args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(),
		Dir: os.Getenv(cpDirEnv),
		Stdio: tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
	}
	os.Exit(cmd.Run(rc, args))
}

func cpHelperCommand(t *testing.T, ctx context.Context, dir string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCpFIFOHelperProcess$")
	child.Env = append(os.Environ(),
		cpHelperEnv+"=1",
		cpDirEnv+"="+dir,
		cpArgsEnv+"="+string(encoded),
	)
	child.Stderr = &stderr
	return child, &stderr
}

func TestCpFIFORecursiveRecreatesNode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(src, 0o731); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o731); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cpTestLimit)
	defer cancel()
	child, stderr := cpHelperCommand(t, ctx, dir, "--preserve=mode", "-R", "fifo", "fifo2")
	if err := child.Run(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("recursive FIFO copy blocked past %s; process was killed", cpTestLimit)
		}
		t.Fatalf("cp failed: %v: %s", err, stderr.String())
	}
	fi, err := os.Lstat(filepath.Join(dir, "fifo2"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("destination type = %v, want FIFO", fi.Mode())
	}
	if fi.Mode().Perm() != 0o731 {
		t.Fatalf("destination mode = %03o, want 731", fi.Mode().Perm())
	}
}

func TestCpRecursiveRecreatesUnixSocket(t *testing.T) {
	// Darwin's sockaddr_un path is short enough that t.TempDir's full
	// per-test name can exceed it.
	dir, err := os.MkdirTemp("/tmp", "cp-socket-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	src := filepath.Join(dir, "socket")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: src, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cpTestLimit)
	defer cancel()
	child, stderr := cpHelperCommand(t, ctx, dir, "-R", "socket", "socket2")
	if err := child.Run(); err != nil {
		if ctx.Err() != nil {
			t.Fatalf("recursive socket copy blocked past %s; process was killed", cpTestLimit)
		}
		t.Fatalf("cp failed: %v: %s", err, stderr.String())
	}
	fi, err := os.Lstat(filepath.Join(dir, "socket2"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSocket == 0 {
		t.Fatalf("destination type = %v, want socket", fi.Mode())
	}
}

func TestCpCopyContentsCreatesRestrictedDirectoryBeforeFIFORead(t *testing.T) {
	for _, tc := range []struct {
		attr       string
		unsafeMask os.FileMode
		finalMode  os.FileMode
	}{
		// --preserve=mode duplicates the source mode exactly; without it
		// the final mode is the source mode modified by the umask (0o022
		// here, pinned around the child spawn below).
		{attr: "mode", unsafeMask: 0o022, finalMode: 0o775},
		{attr: "ownership", unsafeMask: 0o077, finalMode: 0o755},
	} {
		t.Run(tc.attr, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "src"), 0o775); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(dir, "src"), 0o775); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "dest"), 0o2775); err != nil {
				t.Fatal(err)
			}
			fifo := filepath.Join(dir, "src", "fifo")
			if err := unix.Mkfifo(fifo, 0o600); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), cpTestLimit)
			defer cancel()
			child, stderr := cpHelperCommand(t, ctx, dir,
				"--preserve="+tc.attr, "-R", "--copy-contents", "--parents", "src", "dest")
			// The child inherits the file creation mask at spawn; pin it
			// so the umask-filtered final mode is deterministic.
			var startErr error
			withUmask(t, 0o022, func() { startErr = child.Start() })
			if startErr != nil {
				t.Fatal(startErr)
			}
			waited := false
			defer func() {
				if !waited && child.Process != nil {
					_ = child.Process.Signal(syscall.SIGKILL)
					_ = child.Wait()
				}
			}()

			dstDir := filepath.Join(dir, "dest", "src")
			deadline := time.Now().Add(2 * time.Second)
			var fi os.FileInfo
			var err error
			for {
				fi, err = os.Stat(dstDir)
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("destination directory was not created before FIFO read: %v", err)
				}
				time.Sleep(cpPollPeriod)
			}
			if got := fi.Mode().Perm() & tc.unsafeMask; got != 0 {
				t.Fatalf("temporary destination mode %03o has unsafe bits %03o for preserve=%s",
					fi.Mode().Perm(), got, tc.attr)
			}

			// A nonblocking writer succeeds only after cp has opened the
			// FIFO for reading. This makes the producer handshake explicit
			// without risking another indefinitely blocked test goroutine.
			var writer int
			for {
				writer, err = unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK, 0)
				if err == nil {
					break
				}
				if err != unix.ENXIO {
					t.Fatal(err)
				}
				if ctx.Err() != nil {
					t.Fatalf("cp did not open FIFO for reading before %s", cpTestLimit)
				}
				time.Sleep(cpPollPeriod)
			}
			if _, err := unix.Write(writer, []byte("done")); err != nil {
				_ = unix.Close(writer)
				t.Fatal(err)
			}
			if err := unix.Close(writer); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err != nil {
				waited = true
				if ctx.Err() != nil {
					t.Fatalf("copy did not finish after FIFO producer closed; process was killed")
				}
				t.Fatalf("cp failed: %v: %s", err, stderr.String())
			}
			waited = true
			got, err := os.ReadFile(filepath.Join(dstDir, "fifo"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "done" {
				t.Fatalf("copied FIFO contents = %q, want %q", got, "done")
			}
			final, err := os.Stat(dstDir)
			if err != nil {
				t.Fatal(err)
			}
			if final.Mode().Perm() != tc.finalMode {
				t.Fatalf("final destination mode = %03o, want %03o", final.Mode().Perm(), tc.finalMode)
			}
		})
	}
}
