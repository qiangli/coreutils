//go:build unix

package tailcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/qiangli/coreutils/tool"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func TestTailFollowFIFOAcrossWriters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, errb lockedBuffer
	done := make(chan int, 1)
	go func() {
		done <- cmd.Run(&tool.RunContext{
			Ctx: ctx,
			Dir: dir,
			Stdio: tool.Stdio{
				In:  strings.NewReader(""),
				Out: &out,
				Err: &errb,
			},
		}, []string{"-f", "-c", "10", "-s", "0.01", "input.fifo"})
	}()

	write := func(data string) {
		t.Helper()
		wrote := make(chan error, 1)
		go func() { wrote <- os.WriteFile(path, []byte(data), 0o600) }()
		select {
		case err := <-wrote:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("FIFO writer could not rendezvous with tail -f")
		}
	}

	write("first\n")
	// The writer closing does not guarantee the reader has observed the FIFO's
	// writer-less EOF yet. If the second writer connects first, both writes are
	// one 13-byte stream and `-c 10` correctly retains only "st\nsecond\n".
	// Wait for tail to finish and flush the first stream before reconnecting.
	firstDeadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), "first\n") && time.Now().Before(firstDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := out.String(); !strings.Contains(got, "first\n") {
		t.Fatalf("tail did not flush first FIFO writer: stdout=%q stderr=%q", got, errb.String())
	}
	write("second\n")
	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(out.String(), "second\n") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := out.String(); !strings.Contains(got, "first\n") || !strings.Contains(got, "second\n") {
		t.Fatalf("tail did not follow FIFO across writers: stdout=%q stderr=%q", got, errb.String())
	}

	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("tail exit code=%d, stderr=%q", code, errb.String())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("tail did not stop after cancellation")
	}
}
