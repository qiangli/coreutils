// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package kb

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/ref"
)

// writePage is a tiny helper: persist p into the store at dir.
func writePage(t *testing.T, dir string, p *Page) {
	t.Helper()
	if err := Open(dir).Write(p, "add"); err != nil {
		t.Fatal(err)
	}
}

func TestResolveKBFound(t *testing.T) {
	dir := t.TempDir()
	writePage(t, dir, &Page{
		Slug: "deploy-runbook", Type: TypeRunbook, Title: "Deploy runbook",
		Description: "how to ship", Status: StatusValidated,
	})

	g := ref.NewRegistry()
	RegisterRefs(g, dir)

	n, err := g.Resolve("kb:deploy-runbook")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if n.Kind != ref.KB || n.ID != "deploy-runbook" || n.Ref != "kb:deploy-runbook" {
		t.Fatalf("identity = %+v", n)
	}
	if n.Title != "Deploy runbook" {
		t.Errorf("title = %q, want %q", n.Title, "Deploy runbook")
	}
	if n.Status != StatusValidated {
		t.Errorf("status = %q, want %q", n.Status, StatusValidated)
	}
	if n.Open != "bashy kb show deploy-runbook" {
		t.Errorf("open = %q", n.Open)
	}
	if want := "dir " + dir; n.Where != want {
		t.Errorf("where = %q, want %q", n.Where, want)
	}
	if n.Successor != "" {
		t.Errorf("live page has a successor: %q", n.Successor)
	}
}

func TestResolveKBNotFound(t *testing.T) {
	dir := t.TempDir() // readable, empty store
	g := ref.NewRegistry()
	RegisterRefs(g, dir)

	_, err := g.Resolve("kb:nothing-here")
	if !errors.Is(err, ref.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// Edge: a superseded page still resolves, with Status=superseded and Successor
// pointing at the page that replaced it.
func TestResolveKBSupersededCarriesSuccessor(t *testing.T) {
	dir := t.TempDir()
	// The replacement, then the invalidated page linked forward to it — the
	// exact pair `kb supersede` records (old.Status=superseded, old.SupersededBy).
	writePage(t, dir, &Page{
		Slug: "new-way", Type: TypeLesson, Title: "The corrected way",
		Description: "do this", Status: StatusCandidate,
	})
	writePage(t, dir, &Page{
		Slug: "old-way", Type: TypeLesson, Title: "The old way",
		Description: "was wrong", Status: StatusSuperseded, SupersededBy: "new-way",
	})

	g := ref.NewRegistry()
	RegisterRefs(g, dir)

	n, err := g.Resolve("kb:old-way")
	if err != nil {
		t.Fatalf("a superseded page must still resolve: %v", err)
	}
	if n.Status != StatusSuperseded {
		t.Errorf("status = %q, want %q", n.Status, StatusSuperseded)
	}
	if n.Successor != "kb:new-way" {
		t.Errorf("successor = %q, want %q", n.Successor, "kb:new-way")
	}
}

// The default (empty dir) path reads the repo ring of the cwd before the host
// store, exactly as `kb show` auto-detects.
func TestResolveKBRepoRingBeatsHost(t *testing.T) {
	root := kbGitRepo(t) // temp git repo + chdir into it
	host := t.TempDir()
	t.Setenv("BASHY_KB_DIR", host)

	repoDir := filepath.Join(root, RepoSub)
	writePage(t, repoDir, &Page{
		Slug: "shared", Type: TypeFact, Title: "repo copy",
		Description: "d", Status: StatusValidated,
	})
	writePage(t, host, &Page{
		Slug: "shared", Type: TypeFact, Title: "host copy",
		Description: "d", Status: StatusValidated,
	})

	g := ref.NewRegistry()
	RegisterRefs(g, "") // auto: repo ring first, then host

	n, err := g.Resolve("kb:shared")
	if err != nil {
		t.Fatal(err)
	}
	if n.Title != "repo copy" {
		t.Fatalf("title = %q, want the repo-ring copy", n.Title)
	}
	if want := "repo " + repoDir; n.Where != want {
		t.Errorf("where = %q, want %q", n.Where, want)
	}
}
