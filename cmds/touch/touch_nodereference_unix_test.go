//go:build darwin || linux

// Symlink-based -h coverage. os.Symlink needs elevated privileges on Windows,
// and the non-unix setFileTimes fallback refuses symlinks outright, so these
// cases are unix-only by construction.

package touchcmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// lmtime is the modification time of path itself, without following symlinks.
func lmtime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return fi.ModTime()
}

// latime is the access time of path itself, without following symlinks.
func latime(t *testing.T, path string) time.Time {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat %s: %v", path, err)
	}
	return statAtime(fi)
}

// symlinkDir builds dir/target (timed at targetTime) plus dir/link -> target.
func symlinkDir(t *testing.T, targetTime time.Time) (dir, target, link string) {
	t.Helper()
	dir = t.TempDir()
	target = filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, targetTime, targetTime); err != nil {
		t.Fatal(err)
	}
	link = filepath.Join(dir, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Fatal(err)
	}
	return dir, target, link
}

func TestTouchNoDeref(t *testing.T) {
	orig := time.Date(2022, 1, 2, 3, 4, 0, 0, time.Local)

	// Without -h, touch follows the link and retimes the target.
	dir, target, _ := symlinkDir(t, orig)
	if _, errb, code := runTool(t, dir, "-t", "202107081200", "link"); code != 0 {
		t.Fatalf("touch link: code=%d err=%q", code, errb)
	}
	want := time.Date(2021, 7, 8, 12, 0, 0, 0, time.Local)
	if got := mtime(t, target); got.Unix() != want.Unix() {
		t.Errorf("target mtime=%v want %v", got, want)
	}

	// With -h, only the link is retimed; the target keeps its own times.
	dir, target, link := symlinkDir(t, orig)
	if _, errb, code := runTool(t, dir, "-h", "-t", "202107081200", "link"); code != 0 {
		t.Fatalf("touch -h link: code=%d err=%q", code, errb)
	}
	if got := lmtime(t, link); got.Unix() != want.Unix() {
		t.Errorf("-h link mtime=%v want %v", got, want)
	}
	if got := mtime(t, target); got.Unix() != orig.Unix() {
		t.Errorf("-h changed the target: mtime=%v want %v", got, orig)
	}
}

// touch -h -a must leave the symlink's mtime alone, and -h -m its atime.
// Setting both timestamps unconditionally is the easy mistake here.
func TestTouchNoDerefPartialTimes(t *testing.T) {
	orig := time.Date(2015, 5, 6, 7, 8, 0, 0, time.Local)
	dir, _, link := symlinkDir(t, orig)
	if _, errb, code := runTool(t, dir, "-h", "-t", "201505060708", "link"); code != 0 {
		t.Fatalf("seed -h: code=%d err=%q", code, errb)
	}

	// -a: atime moves, mtime must not.
	if _, errb, code := runTool(t, dir, "-h", "-a", "-t", "202001020304", "link"); code != 0 {
		t.Fatalf("-h -a: code=%d err=%q", code, errb)
	}
	wantA := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	if got := latime(t, link); got.Unix() != wantA.Unix() {
		t.Errorf("-h -a atime=%v want %v", got, wantA)
	}
	if got := lmtime(t, link); got.Unix() != orig.Unix() {
		t.Errorf("-h -a clobbered link mtime: got %v want %v", got, orig)
	}

	// -m: mtime moves, atime must not.
	wantM := time.Date(2021, 3, 4, 5, 6, 0, 0, time.Local)
	if _, errb, code := runTool(t, dir, "-h", "-m", "-t", "202103040506", "link"); code != 0 {
		t.Fatalf("-h -m: code=%d err=%q", code, errb)
	}
	if got := lmtime(t, link); got.Unix() != wantM.Unix() {
		t.Errorf("-h -m mtime=%v want %v", got, wantM)
	}
	if got := latime(t, link); got.Unix() != wantA.Unix() {
		t.Errorf("-h -m clobbered link atime: got %v want %v", got, wantA)
	}
}

// A dangling symlink is a valid -h operand: the link exists even though its
// target does not, so touch must retime it rather than create anything.
func TestTouchNoDerefDangling(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	if err := os.Symlink("nowhere", link); err != nil {
		t.Fatal(err)
	}
	if _, errb, code := runTool(t, dir, "-h", "-t", "202001020304", "dangling"); code != 0 {
		t.Fatalf("-h dangling: code=%d err=%q", code, errb)
	}
	want := time.Date(2020, 1, 2, 3, 4, 0, 0, time.Local)
	if got := lmtime(t, link); got.Unix() != want.Unix() {
		t.Errorf("dangling link mtime=%v want %v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "nowhere")); !os.IsNotExist(err) {
		t.Error("-h created the symlink target")
	}
}

func TestTouchTimeWord(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "f")
	orig := time.Date(2010, 1, 1, 0, 0, 0, 0, time.Local)
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(f, orig, orig); err != nil {
		t.Fatal(err)
	}
	// --time=access changes only atime (same as -a)
	_, _, code := runTool(t, dir, "--time=access", "-t", "202107081200", "f")
	if code != 0 {
		t.Fatal("touch --time=access failed")
	}
	// mtime should be unchanged
	if got := mtime(t, f); got.Unix() != orig.Unix() {
		t.Errorf("--time=access changed mtime: %v != %v", got, orig)
	}
	// --time=modify changes only mtime
	_, _, code = runTool(t, dir, "--time=modify", "-t", "202201020304", "f")
	if code != 0 {
		t.Fatal("touch --time=modify failed")
	}
	if got := mtime(t, f); got.Unix() == orig.Unix() {
		t.Errorf("--time=modify did not change mtime")
	}
}
