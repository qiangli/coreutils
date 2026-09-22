package tool

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestModesAreRecorded pins which platform keeps a mode outside the
// filesystem. Getting this wrong in either direction is silent: a false
// here on Windows loses every mode chmod sets, and a true on Unix makes
// every command pay for a lookup that can only return nothing.
func TestModesAreRecorded(t *testing.T) {
	if want := runtime.GOOS == "windows"; ModesAreRecorded != want {
		t.Errorf("ModesAreRecorded = %v on %s", ModesAreRecorded, runtime.GOOS)
	}
}

// TestStatMatchesOS pins that the reader is the platform's own stat plus a
// mode, and nothing else: same size, same name, same identity, same error
// for a path that is not there.
func TestStatMatchesOS(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got.Name() != want.Name() || got.Size() != want.Size() || got.IsDir() != want.IsDir() {
		t.Errorf("Stat reported %v/%d/%v, want %v/%d/%v",
			got.Name(), got.Size(), got.IsDir(), want.Name(), want.Size(), want.IsDir())
	}
	if !SameFile(got, want) {
		t.Error("SameFile says a file is not itself")
	}
	if lgot, lerr := Lstat(path); lerr != nil {
		t.Errorf("Lstat: %v", lerr)
	} else if !SameFile(lgot, got) {
		t.Error("Lstat and Stat of a regular file are not the same file")
	}
	if _, err := Stat(filepath.Join(dir, "absent")); !os.IsNotExist(err) {
		t.Errorf("Stat of a missing file failed with %v, want a not-exist error", err)
	}
	if _, err := Lstat(filepath.Join(dir, "absent")); !os.IsNotExist(err) {
		t.Errorf("Lstat of a missing file failed with %v, want a not-exist error", err)
	}
}

// TestRecordMode pins the writer's contract on a host whose filesystem
// holds the mode: chmod(2) has already done the whole job, so recording is
// a no-op that must not fail — including for a path that does not exist,
// which a caller only reaches after its own chmod succeeded.
func TestRecordMode(t *testing.T) {
	if ModesAreRecorded {
		t.Skip("this host records modes; the round trip is covered in winmode")
	}
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RecordMode(path, 0o644); err != nil {
		t.Errorf("RecordMode: %v", err)
	}
	if err := RecordMode(filepath.Join(t.TempDir(), "absent"), 0o644); err != nil {
		t.Errorf("RecordMode of a missing path: %v", err)
	}
	fi, err := Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if ModeRecorded(fi) {
		t.Error("a host that stores modes itself must report none recorded")
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("mode is %04o, want 0600 — RecordMode must not change it here", got)
	}
}

// TestStatInfoPassesThrough pins that the FileInfo overlay is applied to
// an info obtained elsewhere without disturbing it, and that a nil info
// stays nil rather than becoming a non-nil interface holding nil.
func TestStatInfoPassesThrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, []byte("xy"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	got := StatInfo(path, fi)
	if got.Size() != fi.Size() || got.Name() != fi.Name() {
		t.Error("StatInfo changed something other than the mode")
	}
	if !ModesAreRecorded && got != fs.FileInfo(fi) {
		t.Error("StatInfo must be the identity where the filesystem holds the mode")
	}
	if StatInfo(path, nil) != nil {
		t.Error("StatInfo of a nil FileInfo must stay nil")
	}
}
