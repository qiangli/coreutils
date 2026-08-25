//go:build !windows

package pathchkcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathchkRejectsUnsearchableDirectoryPrefix(t *testing.T) {
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	code, errText := runPathchk(t, dir, "locked/child")
	if code != 1 || !strings.Contains(errText, "not searchable") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkRejectsDanglingSymlinkPrefix(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	code, errText := runPathchk(t, dir, "link/child")
	if code != 1 || !strings.Contains(errText, "cannot access") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}

func TestPathchkPreservesSymlinkBeforeDotDotResolution(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"a", "b", "b/c"} {
		if err := os.Mkdir(filepath.Join(dir, path), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "b", "only"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../b/c", filepath.Join(dir, "a", "link")); err != nil {
		t.Fatal(err)
	}

	code, errText := runPathchk(t, dir, "a/link/../only/child")
	if code != 1 || !strings.Contains(errText, "not a directory") {
		t.Fatalf("code=%d stderr=%q", code, errText)
	}
}
