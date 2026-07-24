//go:build !windows

package ddcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	ddHelperEnv = "COREUTILS_DD_FIFO_HELPER"
	ddArgsEnv   = "COREUTILS_DD_FIFO_ARGS"
	ddDirEnv    = "COREUTILS_DD_FIFO_DIR"
	ddTestLimit = 5 * time.Second
)

func TestDdFIFOHelperProcess(t *testing.T) {
	if os.Getenv(ddHelperEnv) != "1" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(os.Getenv(ddArgsEnv)), &args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	rc := &tool.RunContext{
		Ctx: context.Background(), Dir: os.Getenv(ddDirEnv),
		Stdio: tool.Stdio{In: os.Stdin, Out: os.Stdout, Err: os.Stderr},
	}
	os.Exit(cmd.Run(rc, args))
}

func TestDdFullblockAccumulatesIrregularFIFOReads(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "input")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal([]string{
		"if=input", "of=output", "ibs=8192", "obs=4096",
		"iflag=fullblock", "count=9", "status=noxfer",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddTestLimit)
	defer cancel()
	var stderr bytes.Buffer
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDdFIFOHelperProcess$")
	child.Env = append(os.Environ(), ddHelperEnv+"=1", ddDirEnv+"="+dir, ddArgsEnv+"="+string(encoded))
	child.Stderr = &stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited && child.Process != nil {
			_ = child.Process.Signal(syscall.SIGKILL)
			_ = child.Wait()
		}
	}()

	var fd int
	for {
		fd, err = unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		if err == nil {
			break
		}
		if err != unix.ENXIO {
			t.Fatal(err)
		}
		if ctx.Err() != nil {
			t.Fatalf("dd did not open FIFO before deadline; child killed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	data := bytes.Repeat([]byte("fullblock"), 8192) // 73,728 bytes: exactly 9 input records.
	for off, step := 0, 37; off < len(data); step = (step*7)%997 + 1 {
		end := min(off+step, len(data))
		n, werr := unix.Write(fd, data[off:end])
		if werr == unix.EAGAIN {
			if ctx.Err() != nil {
				_ = unix.Close(fd)
				t.Fatalf("FIFO write exceeded deadline")
			}
			time.Sleep(time.Millisecond)
			continue
		}
		if werr != nil {
			_ = unix.Close(fd)
			t.Fatal(werr)
		}
		if n == 0 {
			if ctx.Err() != nil {
				_ = unix.Close(fd)
				t.Fatalf("FIFO writer made no progress before deadline")
			}
			time.Sleep(time.Millisecond)
			continue
		}
		off += n
	}
	if err := unix.Close(fd); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err != nil {
		waited = true
		if ctx.Err() != nil {
			t.Fatalf("dd blocked past deadline; child killed")
		}
		t.Fatalf("dd failed: %v: %s", err, stderr.String())
	}
	waited = true
	got, err := os.ReadFile(filepath.Join(dir, "output"))
	if err != nil {
		t.Fatal(err)
	}
	want := data
	if !bytes.Equal(got, want) {
		t.Fatalf("output length=%d want=%d", len(got), len(want))
	}
	if wantStatus := "9+0 records in\n18+0 records out\n"; stderr.String() != wantStatus {
		t.Fatalf("status=%q want=%q", stderr.String(), wantStatus)
	}
}
