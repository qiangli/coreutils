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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/qiangli/coreutils/pkg/issue"
	"github.com/qiangli/coreutils/pkg/scope"
)

// Statuses — a personal task's lifecycle. Unlike the committed issue register (which
// delegates "in progress"/"done" to weave so there is one source of truth), a
// steward/human todo OWNS its own execution, so doing/done are first-class here.
const (
	StatusTodo     = "todo"     // not started
	StatusAssigned = "assigned" // delegated to an agent — in that agent's hands (auto-set)
	StatusDoing    = "doing"    // in progress by ME (the steward/human works it directly)
	StatusBlocked  = "blocked"  // waiting on something else
	StatusDone     = "done"     // completed
)

var statuses = []string{StatusTodo, StatusAssigned, StatusDoing, StatusBlocked, StatusDone}

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

// SanitizeOwner reduces an owner to one safe path segment (no traversal),
// defaulting to the steward's list when empty.
func SanitizeOwner(owner string) string {
	if s := scope.SanitizeSegment(owner); s != "" {
		return s
	}
	return DefaultOwner
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

// FindGitRoot walks up from the current directory for a `.git` entry.
func FindGitRoot() (string, bool) { return scope.FindGitRoot() }

// ResolveStore picks the store via the shared scope resolver (the same rule kb
// uses). The DEFAULT (no flag) auto-detects: THIS repo's docs/todo/ inside a git
// repo, else the personal host list. --base-dir travels to another repo, --user
// forces the personal list, --repo forces the repo list. It returns the store
// and a short human label for the scope.
func ResolveStore(owner string, forceRepo, forceUser bool, baseDir string) (*issue.Store, string, error) {
	if owner == "" {
		owner = DefaultOwner
	}
	sc, err := scope.Resolve(scope.Options{
		RepoSub:   RepoSub,
		Owner:     owner,
		HostDir:   Root,
		ForceRepo: forceRepo,
		ForceUser: forceUser,
		BaseDir:   baseDir,
	})
	if err != nil {
		return nil, "", err
	}
	return &issue.Store{Root: sc.Root, Sub: sc.Sub}, sc.Label(), nil
}

// Add files a new todo item (status: todo) into the given store. It files via Save
// directly rather than issue.Add, because the committed issue REGISTER deliberately
// births every issue "open" (triage is a separate act) — a todo, personal or checked
// in, has no triage step, so it is born ready in its own vocabulary.
func Add(st *issue.Store, title, body, priority string, due *time.Time, recurring, assignee string) (*issue.Issue, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("a title is required")
	}
	next := 1
	if m, err := MaxSeq(st); err == nil {
		next = m + 1
	}
	it := &issue.Issue{
		ID:        issue.NewID(),
		Kind:      issue.KindTask,
		Seq:       next,
		Title:     title,
		Body:      body,
		Priority:  priority,
		Due:       due,
		Recurring: recurring,
		Assignee:  assignee,
		Status:    StatusTodo,
		Created:   time.Now().UTC(),
	}
	if _, err := st.Save(it); err != nil {
		return nil, err
	}
	return it, nil
}

// MaxSeq is the highest running number assigned in the store (0 if none).
func MaxSeq(st *issue.Store) (int, error) {
	items, err := st.List()
	if err != nil {
		return 0, err
	}
	max := 0
	for _, it := range items {
		if it.Seq > max {
			max = it.Seq
		}
	}
	return max, nil
}

// EnsureSeq backfills a stable running number onto any items that predate the Seq
// field, in creation order, so a store always shows consistent numbers. Idempotent —
// a no-op once every item has one.
func EnsureSeq(st *issue.Store) error {
	items, err := st.List()
	if err != nil {
		return err
	}
	max := 0
	var missing []*issue.Issue
	for _, it := range items {
		if it.Seq > max {
			max = it.Seq
		}
		if it.Seq == 0 {
			missing = append(missing, it)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Created.Before(missing[j].Created) })
	for _, it := range missing {
		max++
		it.Seq = max
		if _, err := st.Save(it); err != nil {
			return err
		}
	}
	return nil
}

// ResolveRef resolves a reference that may be a running NUMBER (Seq, e.g. "3") or a
// content-id PREFIX (git-style, e.g. "a1148df4"). A bare positive integer is tried as
// a number first — the short handle a human reads off `todo list`.
func ResolveRef(st *issue.Store, ref string) (*issue.Issue, error) {
	ref = strings.TrimSpace(ref)
	if n, err := strconv.Atoi(ref); err == nil && n > 0 {
		items, err := st.List()
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Seq == n {
				return it, nil
			}
		}
		return nil, fmt.Errorf("no task with number %d (run `bashy todo list`)", n)
	}
	return st.Resolve(ref)
}

// SetStatus moves an item to a new status (done stamps Closed).
func SetStatus(st *issue.Store, ref, status string) (*issue.Issue, error) {
	if !ValidStatus(status) {
		return nil, fmt.Errorf("unknown status %q (want one of: %s)", status, strings.Join(statuses, ", "))
	}
	it, err := ResolveRef(st, ref)
	if err != nil {
		return nil, err
	}
	
	if status == StatusDone && it.Recurring != "" {
		it.Status = StatusTodo
		base := time.Now().UTC()
		if it.Due != nil {
			base = *it.Due
		}
		if next, err := advanceCadence(base, it.Recurring); err == nil {
			it.Due = &next
		}
	} else {
		it.Status = status
		if status == StatusDone {
			now := time.Now().UTC()
			it.Closed = &now
		} else {
			it.Closed = nil
		}
	}
	if _, err := st.Save(it); err != nil {
		return nil, err
	}
	return it, nil
}

// Remove drops an item outright.
func Remove(st *issue.Store, ref string) (*issue.Issue, error) {
	it, err := ResolveRef(st, ref)
	if err != nil {
		return nil, err
	}
	return it, st.Remove(it)
}

// List returns a store's items in SEQUENTIAL order — by running number ascending
// (#1, #2, …), i.e. oldest first / creation order — optionally filtered by status.
// The CLI's --reverse flips it to newest-first.
func List(st *issue.Store, status string) ([]*issue.Issue, error) {
	all, err := st.List()
	if err != nil {
		return nil, err
	}
	out := all
	if status != "" {
		out = out[:0:0]
		for _, it := range all {
			if it.Status == status {
				out = append(out, it)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	return out, nil
}

func advanceCadence(from time.Time, cadence string) (time.Time, error) {
	c := strings.ToLower(strings.TrimSpace(cadence))
	switch c {
	case "daily":
		return from.AddDate(0, 0, 1), nil
	case "weekly":
		return from.AddDate(0, 0, 7), nil
	case "monthly":
		return from.AddDate(0, 1, 0), nil
	}
	if d, err := time.ParseDuration(cadence); err == nil {
		return from.Add(d), nil
	}
	sched, err := cron.ParseStandard(cadence)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid recurring cadence %q: %w", cadence, err)
	}
	return sched.Next(from), nil
}
