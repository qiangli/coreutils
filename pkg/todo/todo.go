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

// RepoSub is the committed per-repo todo list's location — home of the SAME record
// format and vocabulary as the personal list, but inside the repo (travels with the
// clone). Deliberately distinct from issue's `.bashy/issues/` (the FORMAL register
// with triage/kinds/weave-linkage): `todo --repo` is the lightweight checked-in list,
// issue is the formal one. One command, two scopes; two levels of formality.
const RepoSub = ".bashy/todo"

// UserStore returns the host-scoped personal store (~/.bashy/todo/<owner>/).
func UserStore(owner string) (*issue.Store, error) {
	root, err := Root()
	if err != nil {
		return nil, err
	}
	return &issue.Store{Root: root, Sub: SanitizeOwner(owner)}, nil
}

// RepoStore returns the committed per-repo store (<repoRoot>/.bashy/todo/).
func RepoStore(repoRoot string) *issue.Store {
	return &issue.Store{Root: repoRoot, Sub: RepoSub}
}

// ResolveStore picks the store for the requested scope. When repo is true, repoRoot
// is consulted to locate the committed list; otherwise the personal owner list is
// used. It returns the store and a short human label for the scope.
func ResolveStore(owner string, repo bool, repoRoot func() (string, error)) (*issue.Store, string, error) {
	if repo {
		if repoRoot == nil {
			return nil, "", fmt.Errorf("--repo is not available here")
		}
		r, err := repoRoot()
		if err != nil {
			return nil, "", err
		}
		return RepoStore(r), "repo " + r, nil
	}
	st, err := UserStore(owner)
	if err != nil {
		return nil, "", err
	}
	return st, "user " + SanitizeOwner(owner), nil
}

// Add files a new todo item (status: todo) into the given store. It files via Save
// directly rather than issue.Add, because the committed issue REGISTER deliberately
// births every issue "open" (triage is a separate act) — a todo, personal or checked
// in, has no triage step, so it is born ready in its own vocabulary.
func Add(st *issue.Store, title, body, priority string) (*issue.Issue, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("a title is required")
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
func SetStatus(st *issue.Store, ref, status string) (*issue.Issue, error) {
	if !ValidStatus(status) {
		return nil, fmt.Errorf("unknown status %q (want one of: %s)", status, strings.Join(statuses, ", "))
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
func Remove(st *issue.Store, ref string) (*issue.Issue, error) {
	it, err := st.Resolve(ref)
	if err != nil {
		return nil, err
	}
	return it, st.Remove(it)
}

// List returns a store's items, newest first, optionally filtered by status.
func List(st *issue.Store, status string) ([]*issue.Issue, error) {
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
