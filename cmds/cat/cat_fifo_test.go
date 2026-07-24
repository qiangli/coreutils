//go:build !windows

package catcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
)

const catFIFOHelperEnv = "COREUTILS_CAT_FIFO_HELPER"

// TestCatFIFOSymlink exercises the same process and FIFO rendezvous used by
// conformance suites. Both sides are bounded: the writer opens the FIFO in
// nonblocking mode, and the parent always kills and waits for a stuck child.
func TestCatFIFOSymlink(t *testing.T) {
	if os.Getenv(catFIFOHelperEnv) == "1" {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		rc := &tool.RunContext{
			Ctx: context.Background(),
			Dir: dir,
			Stdio: tool.Stdio{
				In:  os.Stdin,
				Out: os.Stdout,
				Err: os.Stderr,
			},
		}
		name, args, list := multicall.Resolve("coreutils", []string{"cat", "sympipe"}, "coreutils")
		if list {
			fmt.Fprintln(os.Stderr, "unexpected multicall list request")
			os.Exit(2)
		}
		os.Exit(multicall.Dispatch(rc, name, args))
	}

	dir := t.TempDir()
	pipe := filepath.Join(dir, "dir", "pipe")
	if err := os.Mkdir(filepath.Dir(pipe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(pipe, filepath.Join(dir, "sympipe")); err != nil {
		t.Fatal(err)
	}

	data := bytes.Repeat([]byte("x"), 128*1024)
	var stdout, stderr bytes.Buffer
	child := exec.Command(os.Args[0], "-test.run=^TestCatFIFOSymlink$")
	child.Dir = dir
	child.Env = append(os.Environ(), catFIFOHelperEnv+"=1")
	child.Stdout = &stdout
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}

	waited := false
	waitCh := make(chan error, 1)
	go func() { waitCh <- child.Wait() }()
	defer func() {
		if waited {
			return
		}
		_ = child.Process.Kill()
		<-waitCh
	}()

	writerCh := make(chan error, 1)
	go func() {
		writerCh <- writeFIFOWithDeadline(pipe, data, time.Now().Add(3*time.Second))
	}()

	var childErr, writerErr error
	childDone, writerDone := false, false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !childDone || !writerDone {
		select {
		case childErr = <-waitCh:
			childDone = true
			waited = true
		case writerErr = <-writerCh:
			writerDone = true
		case <-deadline.C:
			_ = child.Process.Kill()
			if !childDone {
				childErr = <-waitCh
				childDone = true
				waited = true
			}
			if !writerDone {
				select {
				case writerErr = <-writerCh:
					writerDone = true
				case <-time.After(time.Second):
					writerErr = errors.New("FIFO writer did not stop after its deadline")
				}
			}
			t.Fatalf("cat FIFO symlink timed out: child=%v writer=%v stderr=%q", childErr, writerErr, stderr.String())
		}
	}
	if writerErr != nil {
		t.Fatalf("FIFO writer: %v (child=%v stderr=%q)", writerErr, childErr, stderr.String())
	}
	if childErr != nil {
		t.Fatalf("cat FIFO symlink: %v; stderr=%q", childErr, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), data) {
		t.Fatalf("cat FIFO symlink output length=%d, want %d", stdout.Len(), len(data))
	}

	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	out, errOut, code := runTool(t, dir, "", "dangling")
	if code != 1 || out != "" || errOut == "" {
		t.Fatalf("cat dangling symlink = stdout %q, stderr %q, code %d; want error", out, errOut, code)
	}
}

func writeFIFOWithDeadline(path string, data []byte, deadline time.Time) error {
	var fd int
	for {
		var err error
		fd, err = syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.ENXIO) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("reader did not open FIFO before deadline: %w", err)
		}
		time.Sleep(time.Millisecond)
	}
	defer syscall.Close(fd)

	for len(data) > 0 {
		n, err := syscall.Write(fd, data)
		if n > 0 {
			data = data[n:]
		}
		if err == nil {
			if n == 0 {
				if time.Now().After(deadline) {
					return errors.New("FIFO write made no progress before deadline")
				}
				time.Sleep(time.Millisecond)
			}
			continue
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EINTR) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("FIFO write did not finish before deadline: %w", err)
		}
		time.Sleep(time.Millisecond)
	}
	return nil
}
