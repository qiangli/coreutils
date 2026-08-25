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

// TestMkdirParentsTrailingSlashFinalMode pins the POSIX pathname rule
// that "x/y/" and "x/y/." name the directory x/y: with -p, the final
// component must receive the -m mode (or the umask default), never the
// u+wx-augmented intermediate mode.
func TestMkdirParentsTrailingSlashFinalMode(t *testing.T) {
	dir := t.TempDir()
	// -m 505 has no owner-write bit; the intermediate mode always does
	// (u+wx is ORed in), so this distinguishes final from intermediate
	// treatment under any umask.
	_, errb, code := runTool(t, dir, "-p", "-m", "505", "x/y/")
	if code != 0 {
		t.Fatalf("mkdir -p -m 505 x/y/: code=%d err=%q", code, errb)
	}
	fi, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o505 {
		t.Errorf("final mode with trailing slash = %o, want 505", got)
	}

	// Without -m, the final directory gets the a=rwx&~umask default, not
	// the (u+wx|~umask) intermediate mode. umask 0327 separates the two:
	// default 0450 vs intermediate-styled 0750.
	old := unix.Umask(0o327)
	defer unix.Umask(old)
	_, errb, code = runTool(t, dir, "-p", "p/q/")
	unix.Umask(old)
	if code != 0 {
		t.Fatalf("mkdir -p p/q/: code=%d err=%q", code, errb)
	}
	qfi, err := os.Stat(filepath.Join(dir, "p", "q"))
	if err != nil {
		t.Fatal(err)
	}
	if got := qfi.Mode().Perm(); got != 0o450 {
		t.Errorf("final default mode with trailing slash = %o, want 450 (umask 0327)", got)
	}

	// "x/y/." is the same directory again: with -p that is ignored
	// without error and without disturbing the mode.
	_, errb, code = runTool(t, dir, "-p", "x/y/.")
	if code != 0 || errb != "" {
		t.Fatalf("mkdir -p x/y/.: code=%d err=%q", code, errb)
	}
	fi2, err := os.Stat(filepath.Join(dir, "x", "y"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi2.Mode().Perm(); got != 0o505 {
		t.Errorf("mode after mkdir -p x/y/. = %o, want 505 unchanged", got)
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
