// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/pkg/kb"
	todopkg "github.com/qiangli/coreutils/pkg/todo"
)

// sprint show --links: a sprint card's prose (spec, acceptance, continuity,
// goal text, thread) may cite kb pages and todos by id — [[kb:slug]],
// [[todo:id]] — and those citations become real at READ time through the one
// resolver kb owns (pkg/kb/links.go; todo show --links calls the same). The
// graph is the kb pages and todo records of every story root the sprint
// tracks (sprint track), plus the current checkout. sprint reads kb and todo;
// neither can see the sprint store (kb is an import leaf), so this is the
// only place a [[sprint:n]] citation's OWN links are resolved, and inbound
// links to a sprint are the ones todo/kb classify as "external".

type sprintLinkRef struct {
	Ref    string `json:"ref"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"` // resolved | dangling | external
	Field  string `json:"field,omitempty"`  // spec | acceptance | continuity | goal | thread
}

// sprintLinkFields is the prose of a card, by field, in display order.
func sprintLinkFields(s *weaveStory) [][2]string {
	fields := [][2]string{
		{"spec", s.SpecRef},
		{"acceptance", s.Acceptance},
		{"continuity", s.Continuity},
	}
	var goal []string
	for _, g := range s.Goal {
		goal = append(goal, g.Text)
	}
	fields = append(fields, [2]string{"goal", strings.Join(goal, "\n")})
	var thread []string
	for _, c := range s.Thread {
		thread = append(thread, c.Body)
	}
	fields = append(fields, [2]string{"thread", strings.Join(thread, "\n")})
	return fields
}

// resolveSprintLinks resolves every citation in the card's prose against the
// kb pages + todo records of the sprint's story roots.
func resolveSprintLinks(s *weaveStory) []sprintLinkRef {
	var nodes []kb.LinkNode
	for _, root := range sprintStoryRoots(s) {
		if pages, err := kb.Open(filepath.Join(root, kb.RepoSub)).List(); err == nil {
			nodes = append(nodes, kb.KBNodes(pages)...)
		}
		if items, err := todopkg.List(todopkg.RepoStore(root), ""); err == nil {
			for _, it := range items {
				nodes = append(nodes, kb.TodoNode(it.ID, it.Title, it.Body))
			}
		}
	}
	var out []sprintLinkRef
	seen := map[string]bool{}
	for _, f := range sprintLinkFields(s) {
		for _, l := range kb.ParseLinks(f[1]) {
			key := f[0] + "|" + l.Ref()
			if seen[key] {
				continue
			}
			seen[key] = true
			switch {
			case l.Kind == kb.LinkSprint:
				out = append(out, sprintLinkRef{Ref: l.Ref(), Status: "external", Field: f[0]})
			default:
				if n, ok := kb.ResolveLink(l, nodes); ok {
					out = append(out, sprintLinkRef{Ref: n.Ref(), Title: n.Title, Status: "resolved", Field: f[0]})
				} else {
					out = append(out, sprintLinkRef{Ref: l.Ref(), Status: "dangling", Field: f[0]})
				}
			}
		}
	}
	return out
}

func renderSprintLinks(w io.Writer, links []sprintLinkRef) {
	fmt.Fprintf(w, "  ── links (%d) ──\n", len(links))
	if len(links) == 0 {
		fmt.Fprintln(w, "  (none — cite a kb page as [[kb:slug]] or a story as [[todo:id]] in the spec, acceptance, continuity, goal or thread)")
		return
	}
	for _, l := range links {
		title := l.Title
		if title == "" {
			title = "-"
		}
		fmt.Fprintf(w, "  -> %-28s %-30s %-9s (%s)\n", l.Ref, title, l.Status, l.Field)
	}
}
