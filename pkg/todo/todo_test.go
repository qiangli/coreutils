// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"testing"
)

func TestTodoLifecycle(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())

	a, err := Add("steward", "wire the webhook", "details", "p1")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != StatusTodo {
		t.Fatalf("new task status = %q, want %q", a.Status, StatusTodo)
	}
	if _, err := Add("steward", "fix CI", "", "p0"); err != nil {
		t.Fatal(err)
	}

	// A different owner keeps a separate list.
	if _, err := Add("fix-42", "someone else's task", "", ""); err != nil {
		t.Fatal(err)
	}
	stew, _ := List("steward", "")
	if len(stew) != 2 {
		t.Fatalf("steward has %d tasks, want 2", len(stew))
	}
	other, _ := List("fix-42", "")
	if len(other) != 1 {
		t.Fatalf("fix-42 has %d tasks, want 1", len(other))
	}

	// Resolve-by-prefix + status transitions.
	if _, err := SetStatus("steward", a.ID[:6], StatusDoing); err != nil {
		t.Fatal(err)
	}
	doing, _ := List("steward", StatusDoing)
	if len(doing) != 1 || doing[0].ID != a.ID {
		t.Fatalf("doing list wrong: %+v", doing)
	}

	done, err := SetStatus("steward", a.ID, StatusDone)
	if err != nil {
		t.Fatal(err)
	}
	if done.Closed == nil {
		t.Fatal("done task must stamp Closed")
	}

	// Remove drops it.
	if _, err := Remove("steward", a.ID); err != nil {
		t.Fatal(err)
	}
	stew, _ = List("steward", "")
	if len(stew) != 1 {
		t.Fatalf("after rm, steward has %d tasks, want 1", len(stew))
	}
}

func TestBadStatusRejected(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	a, _ := Add("steward", "x", "", "")
	if _, err := SetStatus("steward", a.ID, "nope"); err == nil {
		t.Fatal("an unknown status must be rejected")
	}
}

func TestOwnerTraversalIsContained(t *testing.T) {
	if got := SanitizeOwner("../../etc"); got == "../../etc" {
		t.Fatalf("owner traversal not sanitized: %q", got)
	}
}
