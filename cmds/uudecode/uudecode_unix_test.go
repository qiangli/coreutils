//go:build darwin || linux

package uudecodecmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestHeaderModeIgnoresUmask(t *testing.T) {
	old := unix.Umask(0o077)
	t.Cleanup(func() { unix.Umask(old) })
	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "begin 666 out\n \nend\n")
	if code != 0 || errb != "" {
		t.Fatalf("got (%q,%d)", errb, code)
	}
	info, err := os.Stat(filepath.Join(dir, "out"))
	if err != nil || info.Mode().Perm() != 0o666 {
		t.Fatalf("mode=%v err=%v", info.Mode().Perm(), err)
	}
	got := unix.Umask(0)
	unix.Umask(got)
	if got != 0o077 {
		t.Fatalf("uudecode changed process umask to %03o", got)
	}
}

func TestOverwriteFIFODoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pipe")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _, _ = runTool(t, dir, "begin 600 pipe\n \nend\n"); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("uudecode blocked while checking FIFO overwrite")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeNamedPipe != 0 {
		t.Fatalf("FIFO was not atomically replaced: mode=%v err=%v", info.Mode(), err)
	}
}
