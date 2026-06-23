package weave

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitInitCommit(t *testing.T, dir, file, content string) string {
	t.Helper()
	must := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	os.MkdirAll(dir, 0o755)
	must("init", "-q")
	os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644)
	must("add", ".")
	must("commit", "-qm", "c")
	out, _ := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	return strings.TrimSpace(string(out))
}

func TestWeaveSyncSiblingDeps(t *testing.T) {
	tmp := t.TempDir()
	// original siblings next to the source repo
	sh := filepath.Join(tmp, "sh")
	headV1 := gitInitCommit(t, sh, "v.txt", "v1")
	// target repo with go.mod referencing ../sh
	target := filepath.Join(tmp, "bashy")
	gitInitCommit(t, target, "go.mod", "module x\n\nrequire mvdan.cc/sh/v3 v3.0.0\n\nreplace mvdan.cc/sh/v3 => ../sh\n")

	// workspace layout: <queue>/workspaces/issue-1 ; ../sh resolves to <queue>/workspaces/sh
	workspace := filepath.Join(tmp, "queue", "workspaces", "issue-1")
	os.MkdirAll(workspace, 0o755)

	synced, failed := weaveSyncSiblingDeps(target, workspace)
	if len(failed) != 0 {
		t.Fatalf("unexpected failures: %v", failed)
	}
	if len(synced) != 1 || synced[0] != "sh" {
		t.Fatalf("synced = %v, want [sh]", synced)
	}
	dst := filepath.Join(tmp, "queue", "workspaces", "sh")
	if fi, err := os.Stat(filepath.Join(dst, ".git")); err != nil || !fi.IsDir() {
		t.Fatalf("expected git clone at %s", dst)
	}
	gotHead, _ := exec.Command("git", "-C", dst, "rev-parse", "HEAD").Output()
	if strings.TrimSpace(string(gotHead)) != headV1 {
		t.Fatalf("sibling HEAD = %s, want %s", strings.TrimSpace(string(gotHead)), headV1)
	}

	// advance the original sibling; re-sync must update the shared clone (the
	// staleness fix) and discard any stray edit a prior worker left.
	headV2 := gitInitCommit(t, sh, "v.txt", "v2")
	os.WriteFile(filepath.Join(dst, "stray.txt"), []byte("worker junk"), 0o644)
	synced, _ = weaveSyncSiblingDeps(target, workspace)
	if len(synced) != 1 {
		t.Fatalf("re-sync synced = %v", synced)
	}
	gotHead, _ = exec.Command("git", "-C", dst, "rev-parse", "HEAD").Output()
	if strings.TrimSpace(string(gotHead)) != headV2 {
		t.Fatalf("after re-sync HEAD = %s, want %s (stale!)", strings.TrimSpace(string(gotHead)), headV2)
	}
	if _, err := os.Stat(filepath.Join(dst, "stray.txt")); !os.IsNotExist(err) {
		t.Fatalf("stray worker file not cleaned on re-sync")
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "v.txt")); string(b) != "v2" {
		t.Fatalf("sibling content = %q, want v2", b)
	}
}

func TestWeaveSiblingReplaceDirs(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte(
		"module x\nreplace a => ../sh\nreplace (\n\tb => ../coreutils\n\tc => ../readline v1.2.3\n\td => github.com/x/y v1\n)\n"), 0o644)
	got := weaveSiblingReplaceDirs(tmp)
	want := map[string]bool{"../sh": true, "../coreutils": true, "../readline": true}
	if len(got) != 3 {
		t.Fatalf("got %v, want 3 relative replaces", got)
	}
	for _, g := range got {
		if !want[g] {
			t.Fatalf("unexpected replace dir %q in %v", g, got)
		}
	}
}
