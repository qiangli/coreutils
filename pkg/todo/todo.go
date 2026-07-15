// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package todo is the STEWARD'S task list — level 1 of the tracking hierarchy:
//
//	issue  — per repo, COMMITTED backlog (what is wrong/wanted). pkg/issue.
//	sprint — cross-repo, what a conductor is planning now.
//	todo   — per HOST/USER, personal + non-committed. THIS package.
//
// It fills the one level nothing owned: the steward's own running list of what it
// is doing across all repos and all threads — the equivalent of a human's todo-list
// app. A conductor/fixer keeps its own list under its own owner; a human uses the
// default. Same tool, one vocabulary.
//
// It reuses the issue register's record format and store verbatim (YAML-frontmatter
// markdown, content-addressed ids, resolve-by-prefix) — see pkg/issue — but rooted
// at ~/.bashy/todo/<owner>/ (home, not a repo; not committed) and with its own
// status vocabulary. Not a new model or app: the same one at a different scope.
package todo

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/issue"
)

// Statuses — a personal task's lifecycle. Unlike the committed issue register (which
// delegates "in progress"/"done" to weave so there is one source of truth), a
// steward/human todo OWNS its own execution, so doing/done are first-class here.
const (
	StatusTodo    = "todo"    // not started
	StatusDoing   = "doing"   // in progress
	StatusBlocked = "blocked" // waiting on something else
	StatusDone    = "done"    // completed
)

var statuses = []string{StatusTodo, StatusDoing, StatusBlocked, StatusDone}

// ValidStatus reports whether s is a known todo status.
func ValidStatus(s string) bool { return slices.Contains(statuses, s) }

// Statuses returns the status vocabulary.
func Statuses() []string { return append([]string(nil), statuses...) }

// DefaultOwner is the steward's list — the host's on-shift agent.
const DefaultOwner = "steward"

// Root is the host-scoped base directory (~/.bashy/todo). BASHY_TODO_DIR overrides
// it (tests, non-standard homes).
func Root() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_TODO_DIR")); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bashy", "todo"), nil
}

// SanitizeOwner reduces an owner to one safe path segment (no traversal).
func SanitizeOwner(owner string) string {
	owner = strings.TrimSpace(owner)
	owner = strings.ReplaceAll(owner, "..", "-")
	owner = strings.ReplaceAll(owner, "/", "-")
	owner = strings.ReplaceAll(owner, "\\", "-")
	if owner == "" {
		return DefaultOwner
	}
	return owner
}

// Store returns the issue-register store rooted at the owner's host-scoped subtree.
func Store(owner string) (*issue.Store, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	return &issue.Store{Root: root, Sub: SanitizeOwner(owner)}, nil
}

// Add files a new todo item (status: todo) for owner. It files via the store's Save
// directly rather than issue.Add, because the committed register deliberately births
// every issue "open" (triage is a separate act) — a personal todo has no triage step,
// so it is born ready in its own vocabulary.
func Add(owner, title, body, priority string) (*issue.Issue, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("a title is required")
	}
	st, err := Store(owner)
	if err != nil {
		return nil, err
	}
	it := &issue.Issue{
		ID:       issue.NewID(),
		Kind:     issue.KindTask,
		Title:    title,
		Body:     body,
		Priority: priority,
		Status:   StatusTodo,
		Created:  time.Now().UTC(),
	}
	if _, err := st.Save(it); err != nil {
		return nil, err
	}
	return it, nil
}

// SetStatus moves an item to a new status (done stamps Closed).
func SetStatus(owner, ref, status string) (*issue.Issue, error) {
	if !ValidStatus(status) {
		return nil, fmt.Errorf("unknown status %q (want one of: %s)", status, strings.Join(statuses, ", "))
	}
	st, err := Store(owner)
	if err != nil {
		return nil, err
	}
	it, err := st.Resolve(ref)
	if err != nil {
		return nil, err
	}
	it.Status = status
	if status == StatusDone {
		now := time.Now().UTC()
		it.Closed = &now
	} else {
		it.Closed = nil
	}
	if _, err := st.Save(it); err != nil {
		return nil, err
	}
	return it, nil
}

// Remove drops an item outright.
func Remove(owner, ref string) (*issue.Issue, error) {
	st, err := Store(owner)
	if err != nil {
		return nil, err
	}
	it, err := st.Resolve(ref)
	if err != nil {
		return nil, err
	}
	return it, st.Remove(it)
}

// List returns an owner's items, newest first, optionally filtered by status.
func List(owner, status string) ([]*issue.Issue, error) {
	st, err := Store(owner)
	if err != nil {
		return nil, err
	}
	all, err := st.List()
	if err != nil {
		return nil, err
	}
	if status == "" {
		return all, nil
	}
	var out []*issue.Issue
	for _, it := range all {
		if it.Status == status {
			out = append(out, it)
		}
	}
	return out, nil
}
