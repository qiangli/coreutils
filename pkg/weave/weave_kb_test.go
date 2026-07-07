package weave

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/kb"
)

func TestWeaveInjectKBFile(t *testing.T) {
	kbDir := t.TempDir()
	t.Setenv("BASHY_KB_DIR", kbDir)
	store := kb.Open(kbDir)
	if err := store.Write(&kb.Page{
		Slug: "codesign-dance", Type: kb.TypeGotcha,
		Title:       "codesign dance on macOS",
		Description: "WHEN swapping a running signed binary",
		Body:        "rm, cp, codesign --force.",
	}, "add"); err != nil {
		t.Fatal(err)
	}

	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	it := &weaveItem{ID: 1, Title: "fix the codesign failure on upgrade"}
	if err := weaveInjectKBFile("/home/x/.bashy/weave/myrepo-0a1b2c3d", workspace, it); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(workspace, "KB.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.Contains(got, "codesign-dance") {
		t.Fatalf("KB.md missing the matching page:\n%s", got)
	}
	if !strings.Contains(got, "bashy kb retro") {
		t.Fatalf("KB.md missing the write-back instruction:\n%s", got)
	}
	// The drop must be git-excluded so it can never merge.
	excl, err := os.ReadFile(filepath.Join(workspace, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(excl), "KB.md") {
		t.Fatalf("KB.md not git-excluded:\n%s", excl)
	}
}

func TestWeaveInjectKBFileEmptyStore(t *testing.T) {
	t.Setenv("BASHY_KB_DIR", t.TempDir())
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	it := &weaveItem{ID: 2, Title: "anything"}
	if err := weaveInjectKBFile("/home/x/.bashy/weave/myrepo-0a1b2c3d", workspace, it); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(workspace, "KB.md"))
	if err != nil {
		t.Fatal(err)
	}
	// No matches: still carries the contribute + retro instructions.
	if !strings.Contains(string(b), "contribute") || !strings.Contains(string(b), "bashy kb retro") {
		t.Fatalf("empty-store KB.md missing loop instructions:\n%s", b)
	}
}

func TestWeaveRepoNameFromQueueDir(t *testing.T) {
	if got := weaveRepoNameFromQueueDir("/h/.bashy/weave/myrepo-0a1b2c3d"); got != "myrepo" {
		t.Fatalf("got %q", got)
	}
	if got := weaveRepoNameFromQueueDir("/h/.bashy/weave/odd-name"); got != "odd-name" {
		t.Fatalf("got %q", got)
	}
}
