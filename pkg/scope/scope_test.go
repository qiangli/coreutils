// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package scope

import (
	"os"
	"path/filepath"
	"testing"
)

func hostDir(p string) func() (string, error) {
	return func() (string, error) { return p, nil }
}

// gitRepo makes a temp dir a git repo and chdirs into it (auto-restored).
func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	// FindGitRoot resolves the repo from os.Getwd(); compare against the same value
	// so the macOS /var → /private/var symlink does not cause a mismatch.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}

func TestBaseDirWins(t *testing.T) {
	gitRepo(t) // even inside a repo, --base-dir points elsewhere
	sc, err := Resolve(Options{RepoSub: "docs/kb", BaseDir: "/some/other/repo", HostDir: hostDir("/host")})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Kind != KindRepo || sc.Dir() != filepath.Join("/some/other/repo", "docs/kb") {
		t.Fatalf("base-dir not honored: %+v dir=%s", sc, sc.Dir())
	}
}

func TestAutoDetectsGitRepo(t *testing.T) {
	root := gitRepo(t)
	sc, err := Resolve(Options{RepoSub: "docs/todo", HostDir: hostDir("/host")})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Kind != KindRepo {
		t.Fatalf("expected repo scope, got %s", sc.Kind)
	}
	if got := sc.Dir(); got != filepath.Join(root, "docs/todo") {
		t.Fatalf("repo dir = %s, want %s", got, filepath.Join(root, "docs/todo"))
	}
}

func TestForceUserInsideRepo(t *testing.T) {
	gitRepo(t)
	sc, err := Resolve(Options{RepoSub: "docs/kb", Owner: "steward", HostDir: hostDir("/host"), ForceUser: true})
	if err != nil {
		t.Fatal(err)
	}
	if sc.Kind != KindUser || sc.Dir() != filepath.Join("/host", "steward") {
		t.Fatalf("force-user not honored: %+v dir=%s", sc, sc.Dir())
	}
}

func TestForceRepoOutsideRepoErrors(t *testing.T) {
	t.Chdir(t.TempDir()) // a temp dir that is NOT a git repo
	if _, err := Resolve(Options{RepoSub: "docs/kb", ForceRepo: true, HostDir: hostDir("/host")}); err == nil {
		t.Fatal("expected an error forcing --repo outside a git repo")
	}
}

func TestUserScopeNoOwner(t *testing.T) {
	t.Chdir(t.TempDir())
	sc, err := Resolve(Options{RepoSub: "docs/kb", HostDir: hostDir("/host/kb")}) // Owner "" → no subdir (kb shape)
	if err != nil {
		t.Fatal(err)
	}
	if sc.Kind != KindUser || sc.Dir() != "/host/kb" {
		t.Fatalf("host store without owner = %s (%s), want /host/kb", sc.Dir(), sc.Kind)
	}
}

func TestSanitizeSegmentContainsTraversal(t *testing.T) {
	for _, in := range []string{"../../etc", "a/b", `a\b`, "..", "  ../x  "} {
		got := SanitizeSegment(in)
		if got == "" {
			continue
		}
		if filepath.Base(got) != got || got == ".." {
			t.Errorf("SanitizeSegment(%q) = %q — escaped one segment", in, got)
		}
	}
	if SanitizeSegment("   ") != "" {
		t.Error("blank owner should sanitize to empty")
	}
}
