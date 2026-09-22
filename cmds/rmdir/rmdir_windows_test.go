//go:build windows

package rmdircmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestRmdirRecordedReadOnlyDirectory(t *testing.T) {
	base := t.TempDir()
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{"searchable", 0o111},
		{"readable", 0o444},
	} {
		path := filepath.Join(base, tc.name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o444); err != nil {
			t.Fatal(err)
		}
		if err := tool.RecordMode(path, tc.mode); err != nil {
			t.Fatal(err)
		}
		if info, err := tool.Stat(path); err != nil || info.Mode().Perm() != tc.mode {
			t.Fatalf("recorded %s mode: info=%v err=%v", tc.name, info, err)
		}
		if err := os.Remove(path); !os.IsPermission(err) {
			t.Fatalf("raw Windows removal of %s: got %v, want permission denial", tc.name, err)
		}
		_, stderr, code := runTool(t, base, tc.name)
		if code != 0 || stderr != "" {
			t.Errorf("rmdir %s: code=%d stderr=%q", tc.name, code, stderr)
		}
	}
}

func TestRmdirRecordedParentMustBeWritableAndSearchable(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := tool.RecordMode(parent, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tool.RecordMode(parent, 0o700) })
	_, stderr, code := runTool(t, parent, "child")
	if code != 1 || !strings.Contains(stderr, "Permission denied") {
		t.Fatalf("rmdir child of non-writable parent: code=%d stderr=%q", code, stderr)
	}
	if _, err := os.Stat(child); err != nil {
		t.Fatalf("child was removed despite parent mode: %v", err)
	}
}

func TestRmdirDotDotDoesNotClearReadOnlyAttribute(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "a"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(base, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(base, 0o666) })
	_, _, _ = runTool(t, base, "a/..")
	info, err := os.Lstat(base)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o200 != 0 {
		t.Fatal("rmdir a/.. cleared the parent directory's read-only attribute")
	}
}
