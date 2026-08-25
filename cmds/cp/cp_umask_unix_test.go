//go:build unix

package cpcmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"golang.org/x/sys/unix"
)

// withUmask runs fn under the given file creation mask and restores the
// previous mask before returning. The cp test package never calls
// t.Parallel, so the process-global mask cannot leak into another test
// running concurrently.
func withUmask(t *testing.T, mask int, fn func()) {
	t.Helper()
	old := syscall.Umask(mask)
	defer syscall.Umask(old)
	fn()
}

func runToolVirtualUmask(t *testing.T, dir string, mask os.FileMode, args ...string) (string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	rc := &tool.RunContext{
		Ctx:      context.Background(),
		Dir:      dir,
		Stdio:    tool.Stdio{In: strings.NewReader(""), Out: &out, Err: &errb},
		Umask:    mask,
		UmaskSet: true,
	}
	code := cmd.Run(rc, args)
	return errb.String(), code
}

func TestCpRecursiveHonorsVirtualUmaskForAllCreatedTypes(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "file"), "payload")
	if err := unix.Mkfifo(filepath.Join(src, "fifo"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, "file"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(src, "fifo"), 0o666); err != nil {
		t.Fatal(err)
	}

	// Make the host umask deliberately different. The in-process command
	// must use only RunContext.Umask and must not mutate the host mask.
	withUmask(t, 0, func() {
		errOut, code := runToolVirtualUmask(t, dir, 0o027, "-R", "src", "dst")
		if code != 0 || errOut != "" {
			t.Fatalf("cp -R: code=%d err=%q", code, errOut)
		}
	})

	for path, want := range map[string]os.FileMode{
		filepath.Join(dir, "dst"):         0o750,
		filepath.Join(dir, "dst", "file"): 0o640,
		filepath.Join(dir, "dst", "fifo"): 0o640,
	} {
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != want {
			t.Errorf("%s mode=%03o, want %03o", path, got, want)
		}
	}
}

// TestCpRecursiveNewDirUmaskFinalMode pins POSIX cp step 2.d: without -p,
// a newly created destination directory ends up with the SOURCE mode as
// modified by the invoking umask — not the raw source mode.
func TestCpRecursiveNewDirUmaskFinalMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "sub", "file"), "payload")
	// Chmod bypasses the umask, so the source modes are exact.
	for _, p := range []string{filepath.Join(src, "sub"), src} {
		if err := os.Chmod(p, 0o777); err != nil {
			t.Fatal(err)
		}
	}

	withUmask(t, 0o027, func() {
		_, errb, code := runTool(t, dir, "-R", "src", "dst")
		if code != 0 || errb != "" {
			t.Fatalf("cp -R: code=%d err=%q", code, errb)
		}
	})

	for _, p := range []string{filepath.Join(dir, "dst"), filepath.Join(dir, "dst", "sub")} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := fi.Mode().Perm(); got != 0o750 {
			t.Errorf("%s mode = %o, want %o (0777 &^ 0027)", p, got, 0o750)
		}
	}
	if got := read(t, filepath.Join(dir, "dst", "sub", "file")); got != "payload" {
		t.Errorf("tree not populated: %q", got)
	}
}

// TestCpRecursiveNewDirPreserveModeIgnoresUmask: with -p the source mode
// is duplicated exactly; the umask does not modify it.
func TestCpRecursiveNewDirPreserveModeIgnoresUmask(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "file"), "payload")
	if err := os.Chmod(src, 0o775); err != nil {
		t.Fatal(err)
	}

	withUmask(t, 0o077, func() {
		_, errb, code := runTool(t, dir, "-Rp", "src", "dst")
		if code != 0 || errb != "" {
			t.Fatalf("cp -Rp: code=%d err=%q", code, errb)
		}
	})

	fi, err := os.Stat(filepath.Join(dir, "dst"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o775 {
		t.Errorf("dst mode = %o, want %o (preserved, umask ignored)", got, 0o775)
	}
}

// TestCpRecursiveReadOnlySourceDirPopulates: during population the new
// directory carries the umask-filtered mode OR'd with S_IRWXU, so even a
// source directory without owner write produces a populated copy; the
// final mode then drops back to the filtered source mode.
func TestCpRecursiveReadOnlySourceDirPopulates(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "file"), "payload")
	if err := os.Chmod(src, 0o555); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	// Leave every directory writable again so TempDir cleanup succeeds.
	defer func() {
		_ = os.Chmod(src, 0o755)
		_ = os.Chmod(dst, 0o755)
	}()

	withUmask(t, 0o022, func() {
		_, errb, code := runTool(t, dir, "-R", "src", "dst")
		if code != 0 || errb != "" {
			t.Fatalf("cp -R: code=%d err=%q", code, errb)
		}
	})

	if got := read(t, filepath.Join(dst, "file")); got != "payload" {
		t.Errorf("read-only source dir not populated: %q", got)
	}
	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o555 {
		t.Errorf("dst mode = %o, want %o", got, 0o555)
	}
}

// TestCpRecursiveExistingDirKeepsMode: the umask rule applies only to
// directories cp creates; an existing destination directory's mode is
// left alone without -p.
func TestCpRecursiveExistingDirKeepsMode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	write(t, filepath.Join(src, "file"), "payload")
	if err := os.Chmod(src, 0o777); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "dst")
	if err := os.Mkdir(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dst, 0o711); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dst, 0o755) }()

	withUmask(t, 0o022, func() {
		_, errb, code := runTool(t, dir, "-R", "src", "dst")
		if code != 0 || errb != "" {
			t.Fatalf("cp -R into existing dir: code=%d err=%q", code, errb)
		}
	})

	fi, err := os.Stat(filepath.Join(dst, "src"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o755 {
		t.Errorf("dst/src mode = %o, want %o (0777 &^ 0022)", got, 0o755)
	}
	outer, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if got := outer.Mode().Perm(); got != 0o711 {
		t.Errorf("existing dst mode changed: %o, want %o", got, 0o711)
	}
}
