//go:build !windows

package mkdircmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestMkdirParentsRetainOwnerWriteAndSearch(t *testing.T) {
	dir := t.TempDir()
	old := unix.Umask(0o777)
	defer unix.Umask(old)
	defer func() {
		_ = os.Chmod(filepath.Join(dir, "a"), 0o700)
		_ = os.Chmod(filepath.Join(dir, "a", "b"), 0o700)
	}()

	_, errb, code := runTool(t, dir, "-p", filepath.Join("a", "b"))
	if code != 0 {
		t.Fatalf("mkdir -p under restrictive umask: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "a"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm() & 0o300; got != 0o300 {
		t.Fatalf("intermediate owner write/search = %03o, want 300", got)
	}
}

func TestMkdirParentsRejectsDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing-target", filepath.Join(dir, "dangling")); err != nil {
		t.Fatal(err)
	}
	_, errb, code := runTool(t, dir, "-p", "dangling")
	if code != 1 || !strings.Contains(errb, "cannot create directory") {
		t.Fatalf("mkdir -p dangling symlink: code=%d err=%q", code, errb)
	}
}
