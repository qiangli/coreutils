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
