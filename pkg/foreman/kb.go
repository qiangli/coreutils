package foreman

// The host-kb bridge for foreman-driven sessions: the check-before-task
// half of the kb loop is done FOR the agent by prepending the top host-kb
// matches for the session goal to the composed prompt. Computed once per
// session (the goal doesn't change) and empty when nothing matches — a
// missing kb never costs tokens or blocks a session.

import (
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/pkg/kb"
)

func (s *Session) kbPreamble() string {
	if s.kbNote != nil {
		return *s.kbNote
	}
	note := composeKBNote(s.state.Goal)
	s.kbNote = &note
	return note
}

func composeKBNote(goal string) string {
	pages, err := kb.Open("").List()
	if err != nil || len(pages) == 0 {
		return ""
	}
	hits := kb.Search(pages, kb.Query{Terms: kb.Terms(goal), OS: runtime.GOOS})
	if len(hits) == 0 {
		// Deliberately NOT kb.Diagnose here, unlike weave's KB.md. That drop
		// is a file the worker opens when it wants it; this is prepended to
		// the prompt and paid on every session whether or not it is used.
		// Explaining a miss is worth a file and not worth unconditional
		// context — "retrieve tiny" is the budget rule, and a no-match note
		// that costs nothing is the cheapest honest answer.
		return ""
	}
	var b strings.Builder
	b.WriteString("Host kb (shared lessons from all agents on this host — `bashy kb retro` after the task):\n")
	b.WriteString(kb.Renderer{Resolution: kb.ResLine, Bullet: "- ", Sep: " "}.Hits(hits))
	return b.String()
}
