package fanout

import "strings"

// View is a SCOPED read of the board: the shared seed plus only the slice of
// contributions relevant to the reader's lens — never the flat N-way firehose.
// This is the P2 context-pollution mitigation: a flat shared context collapses
// steering accuracy as N grows, so no reader ever sees every other agent's full
// stream.
type View struct {
	Board  string         `json:"board"`
	Seed   string         `json:"seed,omitempty"`
	Refs   []string       `json:"refs,omitempty"`
	Scope  string         `json:"scope,omitempty"`
	Posts  []Contribution `json:"posts"`
	Total  int            `json:"total"`  // live posts before scoping
	Scoped bool           `json:"scoped"` // whether a scope filter was applied
}

// Read returns the seed plus a scoped slice of contributions.
//
//   - scope == ""  → the seed + all live posts (up to limit), unscoped.
//   - scope != ""  → the seed + posts matching the scope: an exact scope-tag
//     match is kept, and the remainder is ranked by term-overlap with the scope
//     string; the top `limit` are returned.
//
// limit <= 0 means "no cap". self is the reader's own author id; its own posts
// are excluded so an instance never re-reads what it just wrote.
func (b *Board) Read(scope, self string, limit int) (View, error) {
	seed, _, refs, err := b.SeedText()
	if err != nil {
		return View{}, err
	}
	posts, err := b.Contributions()
	if err != nil {
		return View{}, err
	}
	// Drop the reader's own posts.
	if self != "" {
		kept := posts[:0:0]
		for _, c := range posts {
			if c.By != self {
				kept = append(kept, c)
			}
		}
		posts = kept
	}
	total := len(posts)

	v := View{Board: b.name, Seed: seed, Refs: refs, Scope: scope, Total: total}

	if strings.TrimSpace(scope) == "" {
		v.Posts = capPosts(posts, limit)
		return v, nil
	}

	v.Scoped = true
	// The scoped view is genuinely NARROWER, not merely reordered — that is the
	// whole point of the context-pollution mitigation. Keep exact scope/tag
	// matches, plus posts with a non-zero term-overlap with the lens; drop the
	// rest entirely (a perf-lens reader never sees the unrelated risk stream).
	var exact, related []Contribution
	for _, c := range posts {
		if c.Scope == scope || hasTag(c.Tags, scope) {
			exact = append(exact, c)
		} else if relevance(c, scope) > 0 {
			related = append(related, c)
		}
	}
	sortByRelevance(related, scope)
	merged := append(exact, related...)
	v.Posts = capPosts(merged, limit)
	return v, nil
}

func capPosts(posts []Contribution, limit int) []Contribution {
	if limit > 0 && len(posts) > limit {
		return posts[:limit]
	}
	return posts
}

func hasTag(tags []string, t string) bool {
	for _, x := range tags {
		if x == t {
			return true
		}
	}
	return false
}

// Render formats a view as the text block an agent reads on the board.
func (v View) Render() string {
	var b strings.Builder
	b.WriteString("=== BOARD: " + v.Board + " ===\n")
	if v.Seed != "" {
		b.WriteString("SEED:\n" + v.Seed + "\n")
	}
	if len(v.Refs) > 0 {
		b.WriteString("REFS: " + strings.Join(v.Refs, ", ") + "\n")
	}
	if len(v.Posts) == 0 {
		b.WriteString("(no contributions yet)\n")
		return b.String()
	}
	which := "all contributions"
	if v.Scoped {
		which = "contributions scoped to " + v.Scope + " (" + itoa(len(v.Posts)) + " of " + itoa(v.Total) + ")"
	}
	b.WriteString("--- " + which + " ---\n")
	for _, c := range v.Posts {
		by := c.By
		if by == "" {
			by = "?"
		}
		b.WriteString("• [" + by + "] " + c.Text + "\n")
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	if neg {
		d = append([]byte{'-'}, d...)
	}
	return string(d)
}
