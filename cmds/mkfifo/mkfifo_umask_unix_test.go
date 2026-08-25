//go:build unix

package mkfifocmd

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMkfifoSymbolicModeHonorsProcessUmask(t *testing.T) {
	old := unix.Umask(0o077)
	defer unix.Umask(old)

	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "-m", "=rw", "pipe")
	if code != 0 {
		t.Fatalf("mkfifo -m =rw: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode=%03o, want 600 under process umask 077", got)
	}
}
