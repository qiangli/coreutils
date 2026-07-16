// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

import (
	"testing"
	"time"
)

func TestParseDue(t *testing.T) {
	// relative days
	t1, err := parseDue("+3d")
	if err != nil || t1 == nil {
		t.Fatalf("failed to parse +3d: %v", err)
	}
	expected := time.Now().UTC().AddDate(0, 0, 3)
	if t1.YearDay() != expected.YearDay() {
		t.Fatalf("expected yearday %d, got %d", expected.YearDay(), t1.YearDay())
	}

	// relative hours
	t2, err := parseDue("+2h")
	if err != nil || t2 == nil {
		t.Fatalf("failed to parse +2h: %v", err)
	}

	// absolute
	t3, err := parseDue("2026-07-20T15:00")
	if err != nil || t3 == nil {
		t.Fatalf("failed to parse absolute: %v", err)
	}
	if t3.Year() != 2026 || t3.Month() != 7 || t3.Day() != 20 {
		t.Fatalf("wrong absolute date: %v", t3)
	}
}

func TestRecurringBehavior(t *testing.T) {
	t.Setenv("BASHY_TODO_DIR", t.TempDir())
	st, _ := UserStore("steward")

	due := time.Now().UTC()
	it, err := Add(st, "daily task", "body", "p1", &due, "daily", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if it.Assignee != "alice" {
		t.Fatalf("expected assignee alice, got %s", it.Assignee)
	}

	// mark done
	it, err = SetStatus(st, it.ID, StatusDone)
	if err != nil {
		t.Fatal(err)
	}

	// should reopen
	if it.Status != StatusTodo {
		t.Fatalf("recurring item should reopen as todo, got %s", it.Status)
	}
	if it.Closed != nil {
		t.Fatalf("recurring item should not be closed")
	}

	// due should be advanced
	if it.Due == nil {
		t.Fatalf("due should not be nil")
	}
	if it.Due.YearDay() != due.AddDate(0, 0, 1).YearDay() {
		t.Fatalf("due not advanced correctly")
	}
}
