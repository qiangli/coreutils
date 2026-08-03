//go:build unix

package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunContextOpenFileVirtualUmask(t *testing.T) {
	dir := t.TempDir()
	rc := &RunContext{Umask: 0o006, UmaskSet: true}
	path := filepath.Join(dir, "new")
	f, err := rc.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o660); got != want {
		t.Fatalf("new file mode=%#o, want %#o", got, want)
	}

	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	f, err = rc.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Fatalf("existing file mode=%#o, want unchanged %#o", got, want)
	}
}
