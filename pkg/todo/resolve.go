// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package todo

// The ref resolver: todo answers `todo:<id>` for the uniform addressing grammar
// (pkg/ref). Registration is one call the embedding shell makes; todo keeps
// resolving only its own kind.
//
// An id is a full 12-hex content id or a git-style unique PREFIX — the same
// convention `todo show` uses (issue.Store.Resolve). The distinction the ref
// contract requires: an id that matches NOTHING is ErrNotFound (a fact about the
// record), but an AMBIGUOUS prefix is a plain error naming the candidates — never
// ErrNotFound, because the record is not absent, the query is under-specified.
// The resolved Node always carries the FULL id, even when a prefix was given.

import (
	"fmt"
	"strings"

	"github.com/qiangli/coreutils/pkg/issue"
	"github.com/qiangli/coreutils/pkg/ref"
)

// RegisterRefs installs the todo resolver on g. The arguments are the same store
// selectors the `todo` CLI takes (see ResolveStore): the store is resolved per
// lookup so the cwd's repo is honored the way the CLI honors it.
func RegisterRefs(g *ref.Registry, owner string, forceRepo, forceUser bool, baseDir string) {
	g.Register(ref.Todo, ref.ResolverFunc(func(id string) (ref.Node, error) {
		st, _, err := ResolveStore(owner, forceRepo, forceUser, baseDir)
		if err != nil {
			return ref.Node{}, err
		}
		return resolveTodo(st, id)
	}))
}

// resolveTodo finds an item by id or unique prefix within one store. It does the
// git-style match itself (rather than issue.Store.Resolve) so it can keep
// "absent" (ErrNotFound) distinct from "ambiguous" (a named error) — the store's
// Resolve collapses both into plain errors.
func resolveTodo(st *issue.Store, id string) (ref.Node, error) {
	id = strings.TrimSpace(id)
	items, err := st.List()
	if err != nil {
		return ref.Node{}, fmt.Errorf("todo: read %s: %w", st.Dir(), err)
	}
	// An exact id always wins over a prefix, exactly as `git show` and the
	// register do.
	for _, it := range items {
		if it.ID == id {
			return todoNode(it, st), nil
		}
	}
	var hits []*issue.Issue
	for _, it := range items {
		if id != "" && strings.HasPrefix(it.ID, id) {
			hits = append(hits, it)
		}
	}
	switch len(hits) {
	case 0:
		return ref.Node{}, fmt.Errorf("todo:%s: %w", id, ref.ErrNotFound)
	case 1:
		return todoNode(hits[0], st), nil
	default:
		names := make([]string, 0, len(hits))
		for _, h := range hits {
			names = append(names, h.ID+" "+h.Title)
		}
		return ref.Node{}, fmt.Errorf("todo:%s is ambiguous — %d items match:\n  %s",
			id, len(hits), strings.Join(names, "\n  "))
	}
}

// todoNode renders an item as a ref.Node. The Node's id is the item's FULL id
// even when a prefix was resolved, so a ref is stable regardless of how it was
// typed.
func todoNode(it *issue.Issue, st *issue.Store) ref.Node {
	n := ref.NewNode(ref.Todo, it.ID)
	n.Title = it.Title
	n.Status = it.Status
	n.Where = st.Dir()
	n.Open = "bashy todo show " + it.ID
	return n
}
