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

// RepoSub is the committed per-repo todo list's location: docs/todo/, inside the repo
// and CHECKED IN. It is the formal, structured replacement for an ad-hoc TODO.md — a
// human- and git-visible directory of one file per item that travels with the clone,
// shows up in diffs, and is the single per-repo tracker (`bashy todo --repo`). Visible
// on purpose (not hidden under .bashy/), because it replaces the file a human reads.
const RepoSub = "docs/todo"

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

// FindGitRoot walks up from the current directory for a `.git` entry (a directory,
// or a file for worktrees/submodules). Returns the repo root and true, or "" and
// false when the cwd is not inside a git repo.
func FindGitRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// ResolveStore picks the store. The DEFAULT (no flag) auto-detects, so an agent can
// just `bashy todo add …` and it lands in the right place; --base-dir lets the same
// agent inspect OTHER repos' lists in one session without cd-ing into them:
//
//	--base-dir <root>  → that project root's list (<root>/docs/todo/). Travel any repo.
//	--user             → force the personal list (~/.bashy/todo/<owner>/), even in a repo
//	default            → THIS repo's docs/todo/ if in a git repo, else the personal list
//	--repo             → force the repo list (errors if not inside a git repo)
//
// It returns the store and a short human label for the scope.
func ResolveStore(owner string, forceRepo, forceUser bool, baseDir string) (*issue.Store, string, error) {
	if b := strings.TrimSpace(baseDir); b != "" {
		return RepoStore(b), "repo " + b, nil
	}
	if !forceUser {
		if root, ok := FindGitRoot(); ok {
			return RepoStore(root), "repo " + root, nil
		}
		if forceRepo {
			return nil, "", fmt.Errorf("--repo: not inside a git repo (a .git was not found here or in any parent)")
		}
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
