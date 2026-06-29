// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

// setupRepo makes a temp dir that looks like a git repo with a .gitignore.
func setupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*.log\nsecret.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSkipNoiseDirs(t *testing.T) {
	root := setupRepo(t)
	m := New(root)
	for _, d := range []string{"node_modules", ".git", "vendor", "dist", "__pycache__"} {
		if !m.Skip(filepath.Join(root, d), true) {
			t.Errorf("expected noise dir %q to be skipped", d)
		}
	}
	if m.Skip(filepath.Join(root, "src"), true) {
		t.Error("a normal source dir must not be skipped")
	}
}

func TestSkipGitignored(t *testing.T) {
	root := setupRepo(t)
	m := New(root)
	if !m.Skip(filepath.Join(root, "debug.log"), false) {
		t.Error("*.log should be gitignored")
	}
	if !m.Skip(filepath.Join(root, "secret.txt"), false) {
		t.Error("secret.txt should be gitignored")
	}
	if m.Skip(filepath.Join(root, "main.go"), false) {
		t.Error("a normal source file must not be skipped")
	}
}

func TestRootNeverSkipped(t *testing.T) {
	// Even if the repo dir's basename is a noise name, the start point itself
	// (rel == ".") is not matched — callers also guard with p != root.
	root := setupRepo(t)
	m := New(root)
	if m.Skip(root, true) { // rel "." — must not match
		t.Error("the repo root itself must not be skipped")
	}
}

func TestHiddenCount(t *testing.T) {
	root := setupRepo(t)
	m := New(root)
	m.Skip(filepath.Join(root, "node_modules"), true)
	m.Skip(filepath.Join(root, "x.log"), false)
	m.Skip(filepath.Join(root, "keep.go"), false) // not skipped
	if m.Hidden() != 2 {
		t.Errorf("Hidden() = %d, want 2", m.Hidden())
	}
}

func TestNilMatcherSkipsNothing(t *testing.T) {
	var m *Matcher // the --agentic-off case
	if m.Skip("/anything", true) {
		t.Error("nil matcher must skip nothing")
	}
	if m.Hidden() != 0 {
		t.Error("nil matcher Hidden() must be 0")
	}
}

func TestNoRepoStillSkipsNoise(t *testing.T) {
	// Outside any git repo, the noise-dir floor still applies; .gitignore layer
	// is simply absent.
	dir := t.TempDir()
	m := New(dir)
	if !m.Skip(filepath.Join(dir, "node_modules"), true) {
		t.Error("noise dirs must be skipped even without a repo")
	}
	if m.Skip(filepath.Join(dir, "anything.log"), false) {
		t.Error("no .gitignore ⇒ no pattern matching")
	}
}
