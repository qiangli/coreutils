//go:build linux

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

func runDdDelayedFIFOCase(
	t *testing.T,
	args []string,
	chunks [][]byte,
	delay time.Duration,
	wantOutput []byte,
	wantStatus string,
) {
	t.Helper()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), ddTestLimit)
	defer cancel()
	var stdout, stderr bytes.Buffer
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDdFIFOHelperProcess$")
	child.Env = append(os.Environ(), ddHelperEnv+"=1", ddDirEnv+"="+dir, ddArgsEnv+"="+string(encoded))
	child.Stdout = &stdout
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
	for _, chunk := range chunks {
		payload := chunk
		for len(payload) > 0 {
			n, werr := unix.Write(fd, payload)
			if werr == unix.EAGAIN || (werr == nil && n == 0) {
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
			payload = payload[n:]
		}
		time.Sleep(delay)
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

	if !bytes.Equal(stdout.Bytes(), wantOutput) {
		t.Fatalf("output=%v want=%v", stdout.Bytes(), wantOutput)
	}
	if stderr.String() != wantStatus {
		t.Fatalf("status=%q want=%q", stderr.String(), wantStatus)
	}
}

func TestDdSyncPadsDelayedFIFORecords(t *testing.T) {
	chunks := make([][]byte, 8)
	want := make([]byte, 8*16)
	for record := 0; record < 8; record++ {
		chunks[record] = bytes.Repeat([]byte{0x0f}, 8)
		for i := 0; i < 8; i++ {
			want[record*16+i] = 0x0f
		}
	}
	runDdDelayedFIFOCase(
		t,
		[]string{"ibs=16", "obs=32", "conv=sync", "if=fifo", "status=noxfer"},
		chunks,
		10*time.Millisecond,
		want,
		"0+8 records in\n4+0 records out\n",
	)
}

func TestDdBsTakesPrecedenceForDelayedFIFORecords(t *testing.T) {
	runDdDelayedFIFOCase(
		t,
		[]string{"bs=3", "ibs=1", "obs=1", "if=fifo"},
		[][]byte{[]byte("ab"), []byte("cd")},
		100*time.Millisecond,
		[]byte("abcd"),
		"0+2 records in\n0+2 records out\n4 bytes copied\n",
	)
}

func TestDdReblocksDelayedPartialFIFORecords(t *testing.T) {
	runDdDelayedFIFOCase(
		t,
		[]string{"ibs=3", "obs=3", "if=fifo"},
		[][]byte{[]byte("ab"), []byte("cd")},
		100*time.Millisecond,
		[]byte("abcd"),
		"0+2 records in\n1+1 records out\n4 bytes copied\n",
	)
}

func runDdFIFOOffsetCase(t *testing.T, args ...string) {
	t.Helper()
	dir := t.TempDir()
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(args)
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
			t.Fatalf("dd did not open FIFO for reading before deadline; child killed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	payload := make([]byte, 512)
	for len(payload) > 0 {
		n, werr := unix.Write(fd, payload)
		if werr == unix.EAGAIN || (werr == nil && n == 0) {
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
		payload = payload[n:]
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
	if want := "0+0 records in\n0+0 records out\n"; stderr.String() != want {
		t.Fatalf("status=%q want=%q", stderr.String(), want)
	}
}

func TestDdSeekOutputFIFOConsumesOutputBlock(t *testing.T) {
	runDdFIFOOffsetCase(t, "count=0", "seek=1", "of=fifo", "status=noxfer")
}

func TestDdSkipInputFIFOConsumesInputBlock(t *testing.T) {
	runDdFIFOOffsetCase(t, "count=0", "skip=1", "if=fifo", "status=noxfer")
}
