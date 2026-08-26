package weave

import (
	"os"
	"path/filepath"
	"testing"
)

// A weave command run from INSIDE a workspace must resolve to the queue that
// created it, not mint a fresh root keyed on the clone's path.
//
// The regression this pins is not hypothetical: ~/.bashy/weave had accumulated
// 237 empty roots (204 named issue-N-<hash>, 31 <repo>-<hash>) because every
// agent that ran a weave command inside its own workspace forked a root. They
// were empty precisely BECAUSE they were wrong — the real queue stayed behind
// in the dispatching root, so nothing ever detected the fork.
func TestWeaveQueueDirResolvesUpwardFromAWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	owner := filepath.Join(weaveStateRoot(home), "coreutils-909dd8b2")
	for _, sub := range []string{"workspaces", "sandboxes"} {
		if err := os.MkdirAll(filepath.Join(owner, sub, "issue-29"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	for _, sub := range []string{"workspaces", "sandboxes"} {
		ws := filepath.Join(owner, sub, "issue-29")
		got, err := weaveQueueDir(ws)
		if err != nil {
			t.Fatalf("weaveQueueDir(%s): %v", sub, err)
		}
		if got != owner {
			t.Fatalf("%s: queue dir = %q, want the owning root %q", sub, got, owner)
		}
	}

	// And an ordinary repo still gets its own root, keyed by basename+hash.
	repo := filepath.Join(home, "projects", "coreutils")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := weaveQueueDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got == owner {
		t.Fatal("an ordinary repo must not resolve to another repo's weave root")
	}
	if filepath.Dir(got) != weaveStateRoot(home) {
		t.Fatalf("ordinary repo root %q is not directly under the state root", got)
	}

	// A directory that merely lives under the state root but is not a workspace
	// (no workspaces/ or sandboxes/ segment) must NOT be swallowed by a root.
	stray := filepath.Join(weaveStateRoot(home), "coreutils-909dd8b2", "logs")
	if err := os.MkdirAll(stray, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, _ := weaveQueueDir(stray); got == owner {
		t.Fatal("a non-workspace path under the state root must not resolve to the owner")
	}
}
