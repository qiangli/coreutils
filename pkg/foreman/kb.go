package foreman

// The host-kb bridge for foreman-driven sessions: the check-before-task
// half of the kb loop is done FOR the agent by prepending the top host-kb
// matches for the session goal to the composed prompt. Computed once per
// session (the goal doesn't change) and empty when nothing matches — a
// missing kb never costs tokens or blocks a session.

import (
	"fmt"
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
		return ""
	}
	var b strings.Builder
	b.WriteString("Host kb (shared lessons from all agents on this host — `bashy kb retro` after the task):\n")
	for _, h := range hits {
		p := h.Page
		fmt.Fprintf(&b, "- %s [%s/%s] %s — %s\n", p.Slug, p.Status, p.Type, p.Title, p.Description)
	}
	return b.String()
}
