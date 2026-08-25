//go:build linux

package ddcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestDdLinuxFIFOImmediateWriterOpenCloseBeforeFirstReadStress proves the
// Linux-specific fact the input state machine relies on: POLLHUP remains
// observable when a writer opens and closes after dd's non-blocking read open
// but before dd's first read. There are deliberately no sleeps in the writer
// path; each iteration creates exactly that lost-before-first-read race.
func TestDdLinuxFIFOImmediateWriterOpenCloseBeforeFirstReadStress(t *testing.T) {
	dir := t.TempDir()
	sigctx := newInterruptContext()
	defer sigctx.Stop()

	const iterations = 500
	for i := range iterations {
		path := filepath.Join(dir, fmt.Sprintf("fifo-%d", i))
		if err := unix.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		f, fifo, err := interruptibleOpenRead(path, sigctx)
		if err != nil {
			t.Fatalf("iteration %d: open read: %v", i, err)
		}
		if !fifo {
			t.Fatalf("iteration %d: FIFO not recognized", i)
		}
		r, ok := newInterruptReader(f, sigctx, true)
		if !ok {
			_ = f.Close()
			t.Fatalf("iteration %d: FIFO reader was not wrapped", i)
		}

		writer, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			_ = r.Close()
			_ = f.Close()
			t.Fatalf("iteration %d: immediate writer open: %v", i, err)
		}
		if err := unix.Close(writer); err != nil {
			t.Fatalf("iteration %d: immediate writer close: %v", i, err)
		}

		type readResult struct {
			n   int
			err error
		}
		result := make(chan readResult, 1)
		go func() {
			n, err := r.Read(make([]byte, 1))
			result <- readResult{n: n, err: err}
		}()
		select {
		case got := <-result:
			if got.n != 0 || got.err != io.EOF {
				t.Fatalf("iteration %d: read=(%d, %v), want (0, EOF)", i, got.n, got.err)
			}
		case <-time.After(time.Second):
			_ = f.Close()
			t.Fatalf("iteration %d: lost immediate writer transition", i)
		}
		_ = r.Close()
		if err := f.Close(); err != nil {
			t.Fatalf("iteration %d: close read: %v", i, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatalf("iteration %d: remove FIFO: %v", i, err)
		}
	}
}
