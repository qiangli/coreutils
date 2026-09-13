// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/kb"
	todopkg "github.com/qiangli/coreutils/pkg/todo"
)

// TestResolveSprintLinks: a card's prose cites kb pages and todos by id and
// they resolve at read time against the tracked story root; a [[sprint:n]]
// citation is external (the sprint store is not a link node); an unknown
// slug is dangling, never an error.
func TestResolveSprintLinks(t *testing.T) {
	root := t.TempDir()
	st := kb.Open(filepath.Join(root, kb.RepoSub))
	if err := st.Write(&kb.Page{Form: kb.FormNote, Type: kb.TypeLesson, Title: "Deploy runbook", Slug: "deploy-runbook", Body: "x", Status: kb.StatusCandidate}, "add"); err != nil {
		t.Fatal(err)
	}
	it, err := todopkg.Add(todopkg.RepoStore(root), "Fix the gate", "", "", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	s := &weaveStory{
		ID:         1,
		SpecRef:    "[[kb:deploy-runbook]]",
		Acceptance: "done when [[todo:" + it.ID[:8] + "]] closes; see [[sprint:2]] and [[kb:nope]]",
		StoryRoots: []string{root},
	}
	got := map[string]string{}
	for _, l := range resolveSprintLinks(s) {
		got[l.Ref] = l.Status + "/" + l.Field
	}
	want := map[string]string{
		"kb:deploy-runbook": "resolved/spec",
		"todo:" + it.ID:     "resolved/acceptance",
		"sprint:2":          "external/acceptance",
		"kb:nope":           "dangling/acceptance",
	}
	for ref, w := range want {
		if got[ref] != w {
			t.Errorf("%s: %q, want %q (all: %v)", ref, got[ref], w, got)
		}
	}
}
