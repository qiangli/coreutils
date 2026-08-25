//go:build unix

package mkfifocmd

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// POSIX Issue 7: without -m the FIFO is created with a=rw (0666) modified
// by the file mode creation mask.  UmaskSet=false exercises the standalone
// process-owned path where mkfifo(2) applies the process umask itself.
func TestMkfifoDefaultModeHonorsProcessUmask(t *testing.T) {
	old := unix.Umask(0o027)
	defer unix.Umask(old)

	dir := t.TempDir()
	_, errb, code := runTool(t, dir, "pipe")
	if code != 0 {
		t.Fatalf("mkfifo: code=%d err=%q", code, errb)
	}
	fi, err := os.Lstat(filepath.Join(dir, "pipe"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o640 {
		t.Fatalf("default mode=%03o, want 640 under process umask 027", got)
	}
}

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
