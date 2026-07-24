package rootguard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameFileIdentity(t *testing.T) {
	reference := t.TempDir()
	child := filepath.Join(reference, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}

	alias := child + string(filepath.Separator) + ".."
	if !SameFile(alias, reference, true) {
		t.Fatal("lexical alias was not recognized by filesystem identity")
	}
	if SameFile(child, reference, true) {
		t.Fatal("different directory was classified as the reference")
	}
}

func TestSameFileFinalSymlinkPolicy(t *testing.T) {
	dir := t.TempDir()
	reference := filepath.Join(dir, "reference")
	if err := os.Mkdir(reference, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(reference, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	if !SameFile(link, reference, true) {
		t.Fatal("followed final symlink was not recognized")
	}
	if SameFile(link, reference, false) {
		t.Fatal("unfollowed final symlink was classified as its referent")
	}
}

func TestRootPathAndAliasSuffix(t *testing.T) {
	root := string(filepath.Separator)
	path := filepath.Join(root, "var", "tmp", "fixture")
	if got := RootPath(path); got != root {
		t.Fatalf("RootPath(%q)=%q, want %q", path, got, root)
	}
	if got := AliasSuffix(root, path); got != "" {
		t.Fatalf("literal-root suffix=%q, want empty", got)
	}
	wantSuffix := " (same as '" + root + "')"
	if got := AliasSuffix(root+"var"+string(filepath.Separator)+"..", path); got != wantSuffix {
		t.Fatalf("alias suffix=%q, want %q", got, wantSuffix)
	}
}
