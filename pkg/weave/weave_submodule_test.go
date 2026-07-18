// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func gitIdent(t *testing.T, dir string) {
	gitRun(t, dir, "config", "user.email", "t@example.com")
	gitRun(t, dir, "config", "user.name", "weave-test")
}

// TestWeaveHydrateSubmodulesFromLocalOrigin reproduces the coreutils self-repair
// blocker in miniature: a `git clone --local` leaves submodules EMPTY, and a repo
// whose build reaches into a submodule can't compile until it is hydrated.
func TestWeaveHydrateSubmodulesFromLocalOrigin(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()

	// 1. A submodule origin repo (stands in for external/ollama/src).
	subOrigin := filepath.Join(base, "suborigin")
	mustMkdir(t, subOrigin)
	gitRun(t, subOrigin, "init", "-q")
	gitIdent(t, subOrigin)
	mustWrite(t, filepath.Join(subOrigin, "go.mod"), "module example.com/sub\n\ngo 1.21\n")
	gitRun(t, subOrigin, "add", ".")
	gitRun(t, subOrigin, "commit", "-qm", "init sub")

	// 2. A superproject origin that vendors the submodule (populated working tree).
	superOrigin := filepath.Join(base, "superorigin")
	mustMkdir(t, superOrigin)
	gitRun(t, superOrigin, "init", "-q")
	gitIdent(t, superOrigin)
	mustWrite(t, filepath.Join(superOrigin, "root.txt"), "x")
	gitRun(t, superOrigin, "add", ".")
	gitRun(t, superOrigin, "commit", "-qm", "root")
	gitRun(t, superOrigin, "-c", "protocol.file.allow=always", "submodule", "add", subOrigin, "vendor/sub")
	gitRun(t, superOrigin, "commit", "-qm", "add sub")

	// 3. `git clone --local` — exactly how a weave workspace is made. The
	//    submodule is left EMPTY (the bug).
	workspace := filepath.Join(base, "workspace")
	gitRun(t, base, "clone", "--local", "--no-hardlinks", superOrigin, workspace)
	if entries, _ := os.ReadDir(filepath.Join(workspace, "vendor", "sub")); len(entries) != 0 {
		t.Fatalf("precondition failed: submodule should be empty after --local clone, got %d entries", len(entries))
	}

	// 4. Hydrate from the local origin.
	if err := weaveHydrateSubmodules(superOrigin, workspace, io.Discard, io.Discard); err != nil {
		t.Fatalf("hydrate: %v", err)
	}

	// 5. The submodule working tree is now populated — the build can see go.mod.
	if _, err := os.Stat(filepath.Join(workspace, "vendor", "sub", "go.mod")); err != nil {
		t.Fatalf("submodule not hydrated: %v", err)
	}
	// A prior interrupted hydration can leave untracked nested artifacts.  The
	// next pre-agent hydration must restore a clean workspace, otherwise a later
	// wrapper auto-commit fails despite no agent source change.
	mustWrite(t, filepath.Join(workspace, "vendor", "sub", "hydration.tmp"), "scratch\n")
	if got := string(gitT(t, workspace, "status", "--porcelain")); got == "" {
		t.Fatal("precondition: nested artifact must make superproject dirty")
	}
	if err := weaveHydrateSubmodules(superOrigin, workspace, io.Discard, io.Discard); err != nil {
		t.Fatalf("re-hydrate clean: %v", err)
	}
	if got := string(gitT(t, workspace, "status", "--porcelain")); got != "" {
		t.Fatalf("hydration left workspace dirty: %q", got)
	}

	// 6. Isolation: the URL was synced back to its canonical .gitmodules value, not
	//    left pointing at the superproject-origin path (a breadcrumb an escaping
	//    agent could follow back to the source checkout).
	override := filepath.Join(superOrigin, "vendor", "sub")
	out, _ := exec.Command("git", "-C", workspace, "config", "submodule.vendor/sub.url").Output()
	if got := string(out); len(got) > 0 && filepath.Clean(trimNL(got)) == filepath.Clean(override) {
		t.Fatalf("submodule url left pointing at the local origin override %q — isolation breadcrumb", override)
	}
}

// A repo with no .gitmodules is a clean no-op.
func TestWeaveHydrateSubmodulesNoGitmodules(t *testing.T) {
	ws := t.TempDir()
	if err := weaveHydrateSubmodules(ws, ws, io.Discard, io.Discard); err != nil {
		t.Fatalf("expected no-op for a repo without submodules, got %v", err)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
