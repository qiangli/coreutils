//go:build unix

package atcmd

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func allowAtForTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "at.allow"), []byte(current.Username+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := atAccessDirs
	atAccessDirs = []string{dir}
	t.Cleanup(func() { atAccessDirs = old })
}

func TestAtAccessAllowTakesPrecedenceAndDenyRejects(t *testing.T) {
	dir := t.TempDir()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	old := atAccessDirs
	atAccessDirs = []string{dir}
	t.Cleanup(func() { atAccessDirs = old })

	if err := os.WriteFile(filepath.Join(dir, "at.allow"), []byte("someone-else\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runATNoStdin(t, context.Background(), "-l")
	if code != 1 || stderr == "" {
		t.Fatalf("allow precedence: code=%d stderr=%q", code, stderr)
	}

	if err := os.Remove(filepath.Join(dir, "at.allow")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "at.deny"), []byte(current.Username+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code = runATNoStdin(t, context.Background(), "-l")
	if code != 1 || stderr == "" {
		t.Fatalf("deny match: code=%d stderr=%q", code, stderr)
	}
}
