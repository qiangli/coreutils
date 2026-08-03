//go:build unix

package diffcmd

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// POSIX diff assertion 42 compares directory entries with the same name but
// different types.  In particular, it stages a FIFO without a writer opposite
// a regular file.  Directory comparison must report the types, not try to read
// the FIFO and hang.
func TestDirectoryFIFOComparedByType(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"left", "right"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fifo := filepath.Join(dir, "left", "node")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "right"), "node", "payload\n")

	// A nonblocking helper also makes this regression bounded against the old
	// implementation: if diff incorrectly opens the FIFO for reading, the
	// helper supplies data so the test fails on output instead of deadlocking.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			fd, err := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK, 0)
			if err == nil {
				_, _ = unix.Write(fd, []byte("payload\n"))
				_ = unix.Close(fd)
				return
			}
			if !errors.Is(err, unix.ENXIO) {
				return
			}
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()

	out, errb, code := runIn(t, dir, "", "left", "right")
	close(stop)
	<-done
	want := "File left/node is a named pipe while file right/node is a regular file\n"
	if code != 1 || out != want || errb != "" {
		t.Fatalf("diff directories with FIFO = (%q, %q, %d), want (%q, %q, 1)",
			out, errb, code, want, "")
	}
}
