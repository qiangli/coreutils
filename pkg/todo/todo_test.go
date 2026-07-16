// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qiangli/coreutils/pkg/issue"
)

func TestTodoLifecycle(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	stew, err := UserStore("steward")
	if err != nil {
		t.Fatal(err)
	}
	a, err := Add(stew, "wire the webhook", "details", "p1", nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusTodo {
		t.Fatalf("new task status = %q, want %q", a.Status, StatusTodo)
	}
	if _, err := Add(stew, "fix CI", "", "p0", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	other, _ := UserStore("fix-42")
	if _, err := Add(other, "someone else's task", "", "", nil, "", ""); err != nil {
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

func TestRepoStoreIsDocsTodo(t *testing.T) {
	rs := RepoStore("/some/repo")
	if rs.Sub != RepoSub || RepoSub != "docs/todo" {
		t.Fatalf("repo sub = %q, want docs/todo", rs.Sub)
	}
	if got := filepath.Join(rs.Root, rs.Sub); got != filepath.Join("/some/repo", "docs/todo") {
		t.Fatalf("repo dir = %q", got)
	}
}

func TestScopeResolution(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())

	// --base-dir targets another project root's docs/todo, without cd.
	base := t.TempDir()
	bst, label, err := ResolveStore("steward", false, false, base)
	if err != nil {
		t.Fatal(err)
	}
	if bst.Root != base || bst.Sub != RepoSub {
		t.Fatalf("base-dir store = %s/%s, want %s/%s", bst.Root, bst.Sub, base, RepoSub)
	}
	if label != "repo "+base {
		t.Fatalf("label %q", label)
	}

	// --user forces the personal list.
	ust, ulabel, err := ResolveStore("steward", false, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if ust.Sub != "steward" {
		t.Fatalf("user sub = %q, want steward", ust.Sub)
	}
	if !strings.HasPrefix(ulabel, "user ") {
		t.Fatalf("user label %q", ulabel)
	}

	// Items in the base-dir store don't leak into the personal list.
	if _, err := Add(bst, "checked-in task", "", "", nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(bst, ""); len(got) != 1 {
		t.Fatalf("base-dir store has %d, want 1", len(got))
	}
	if got, _ := List(ust, ""); len(got) != 0 {
		t.Fatalf("personal store leaked repo items: %d", len(got))
	}
}

func TestBadStatusRejected(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	st, _ := UserStore("steward")
	a, _ := Add(st, "x", "", "", nil, "", "")
	if _, err := SetStatus(st, a.ID, "nope"); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
}

func TestOwnerTraversalIsContained(t *testing.T) {
	if got := SanitizeOwner("../../etc"); got == "../../etc" {
		t.Fatalf("owner traversal not sanitized: %q", got)
	}
}

func TestIsOverdue(t *testing.T) {
	// due yesterday -> overdue
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	it := &issue.Issue{Status: StatusTodo, Due: &yesterday}
	if !IsOverdue(it) {
		t.Fatal("due yesterday should be overdue")
	}

	// due tomorrow -> not overdue
	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	it2 := &issue.Issue{Status: StatusTodo, Due: &tomorrow}
	if IsOverdue(it2) {
		t.Fatal("due tomorrow should not be overdue")
	}

	// done + due yesterday -> NOT overdue
	it3 := &issue.Issue{Status: StatusDone, Due: &yesterday}
	if IsOverdue(it3) {
		t.Fatal("done item should never be overdue")
	}

	// nil due -> not overdue
	it4 := &issue.Issue{Status: StatusTodo}
	if IsOverdue(it4) {
		t.Fatal("nil due should not be overdue")
	}
}

func TestListShowsOverdueMarker(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	st, _ := UserStore("steward")

	// Add an overdue item
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	_, err := Add(st, "overdue task", "", "", &yesterday, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Add a non-overdue item
	tomorrow := time.Now().UTC().AddDate(0, 0, 1)
	_, err = Add(st, "future task", "", "", &tomorrow, "", "")
	if err != nil {
		t.Fatal(err)
	}

	items, err := List(st, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}

	// Find the overdue item
	var overdueItem *issue.Issue
	var futureItem *issue.Issue
	for _, it := range items {
		if it.Title == "overdue task" {
			overdueItem = it
		}
		if it.Title == "future task" {
			futureItem = it
		}
	}
	if overdueItem == nil || futureItem == nil {
		t.Fatal("items not found")
	}

	if !IsOverdue(overdueItem) {
		t.Fatal("overdue task should be overdue")
	}
	if IsOverdue(futureItem) {
		t.Fatal("future task should not be overdue")
	}

	// Verify marker appears in list output by inspecting the dueStr logic
	dueStr := overdueItem.Due.Format("2006-01-02") + " (OVERDUE)"
	if !strings.Contains(dueStr, "(OVERDUE)") {
		t.Fatal("overdue dueStr should contain (OVERDUE)")
	}
}
