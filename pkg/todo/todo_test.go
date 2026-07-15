// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"path/filepath"
	"testing"
)

func TestTodoLifecycle(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	stew, err := UserStore("steward")
	if err != nil {
		t.Fatal(err)
	}
	a, err := Add(stew, "wire the webhook", "details", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusTodo {
		t.Fatalf("new task status = %q, want %q", a.Status, StatusTodo)
	}
	if _, err := Add(stew, "fix CI", "", "p0"); err != nil {
		t.Fatal(err)
	}
	other, _ := UserStore("fix-42")
	if _, err := Add(other, "someone else's task", "", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(stew, ""); len(got) != 2 {
		t.Fatalf("steward has %d tasks, want 2", len(got))
	}
	if got, _ := List(other, ""); len(got) != 1 {
		t.Fatalf("fix-42 has %d tasks, want 1", len(got))
	}
	if _, err := SetStatus(stew, a.ID[:6], StatusDoing); err != nil {
		t.Fatal(err)
	}
	doing, _ := List(stew, StatusDoing)
	if len(doing) != 1 || doing[0].ID != a.ID {
		t.Fatalf("doing list wrong: %+v", doing)
	}
	done, err := SetStatus(stew, a.ID, StatusDone)
	if err != nil {
		t.Fatal(err)
	}
	if done.Closed == nil {
		t.Fatal("done task must stamp Closed")
	}
	if _, err := Remove(stew, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(stew, ""); len(got) != 1 {
		t.Fatalf("after rm, steward has %d tasks, want 1", len(got))
	}
}

func TestScopeResolution(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	repoRoot := t.TempDir()
	rst, label, err := ResolveStore("steward", true, func() (string, error) { return repoRoot, nil })
	if err != nil {
		t.Fatal(err)
	}
	if rst.Sub != RepoSub || rst.Root != repoRoot {
		t.Fatalf("repo store = %s/%s, want %s/%s", rst.Root, rst.Sub, repoRoot, RepoSub)
	}
	if label != "repo "+repoRoot {
		t.Fatalf("repo label = %q", label)
	}
	if _, err := Add(rst, "checked-in task", "", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(rst, ""); len(got) != 1 {
		t.Fatalf("repo store has %d, want 1", len(got))
	}
	ust, _, _ := ResolveStore("steward", false, nil)
	if got, _ := List(ust, ""); len(got) != 0 {
		t.Fatalf("personal store leaked repo items: %d", len(got))
	}
	if got := filepath.Join(rst.Root, rst.Sub); got != filepath.Join(repoRoot, RepoSub) {
		t.Fatalf("repo dir = %q", got)
	}
}

func TestBadStatusRejected(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	st, _ := UserStore("steward")
	a, _ := Add(st, "x", "", "")
	if _, err := SetStatus(st, a.ID, "nope"); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
}

func TestOwnerTraversalIsContained(t *testing.T) {
	if got := SanitizeOwner("../../etc"); got == "../../etc" {
		t.Fatalf("owner traversal not sanitized: %q", got)
	}
}
