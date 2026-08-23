//go:build !windows

package sedcmd

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSedWriteKeepsFIFOOpenFromPreparationThroughExecution(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	read := make(chan []byte, 1)
	go func() {
		data, _ := os.ReadFile(path)
		read <- data
	}()

	run := make(chan struct{})
	var out, errOut string
	var code int
	go func() {
		out, errOut, code = runSedInDir(t, dir, "A\nB\n", "-n", "w capture")
		close(run)
	}()
	select {
	case <-run:
	case <-time.After(3 * time.Second):
		t.Fatal("sed reopened the FIFO after its preparation writer was consumed")
	}
	if code != 0 || errOut != "" || out != "" {
		t.Fatalf("FIFO w command = (%q, %q, %d)", out, errOut, code)
	}
	select {
	case got := <-read:
		if string(got) != "A\nB\n" {
			t.Fatalf("FIFO contents=%q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("FIFO reader did not observe writer close")
	}
}
