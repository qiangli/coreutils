package craft

// RESOLVE — ask for a capability in your own words.
//
// The catalog is addressed by NAME today: you must already know a skill is
// called `go-repo-health` to reach it. That is a 1980s API for a knowledge
// base, and it is also the mechanism behind the failure mode this whole design
// exists to remove.
//
// Skill shadowing is, definitionally, a SELECTION FAILURE AMONG NAMED PEERS:
// the model is handed a list and picks the wrong row, and the more
// semantically-overlapping rows there are, the worse it gets. Capability keys
// already stop variants from being peers. Query resolution removes the list.
// Together the failure is not mitigated but unrepresentable — there is no menu
// to mis-pick from, because the answer is composed rather than chosen.
//
// # The floor never needs a model
//
// Retrieval degrades in tiers, and the bottom tier is stdlib-only:
//
//	tier 0  field-weighted lexical scoring + GRAPH EXPANSION
//	tier 1  TF-IDF over the corpus            (not yet built)
//	tier 2  embeddings, injected              (not yet built)
//
// Tier 0 does more than it sounds like, because of the expansion: a lexical hit
// on one node drags in its typed neighbourhood — the other implementations of
// the same capability, the skills that compose it. That supplies much of what
// people reach for embeddings to get, at zero dependency cost.
//
// Standalone-first is not negotiable here: every gate and every test runs at the
// floor, so results are reproducible. A tier that needs a network is an
// accelerator, never a requirement.
//
// # Name is a matcher, not a key
//
// An exact name match is simply the highest-precedence signal inside Resolve,
// which is what keeps `bashy conductor` and bare-name dispatch working while the
// primary interface becomes a question.

import (
	"sort"
	"strings"
	"unicode"

	dhntskills "github.com/dhnt/dhnt/skills"

	"github.com/qiangli/coreutils/pkg/skills"
)

// Implementation is one skill under a capability.
type Implementation struct {
	Name     string
	Identity string
	Skill    dhntskills.Skill
	// Description is the prose one-liner, when the catalog carries one.
	Description string
	// Bindings map contract predicates and step primitives to concrete
	// commands. Their presence is what makes a band-0 rendering possible.
	Bindings map[string]string
}

// Capability groups the implementations that make one guarantee.
type Capability struct {
	Key             string
	Implementations []Implementation
}

// Match is one scored result.
type Match struct {
	Capability *Capability `json:"-"`
	Key        string      `json:"capability"`
	// Primary is the elected implementation for this coordinate.
	Primary Implementation `json:"-"`
	Name    string         `json:"name"`
	Score   float64        `json:"score"`
	// Why names the signals that fired, so a ranking can be explained rather
	// than trusted. A retrieval system nobody can interrogate is one nobody can
	// debug when it starts returning the wrong thing.
	Why []string `json:"why,omitempty"`
	// Alternatives counts the other implementations of the same guarantee.
	Alternatives int `json:"alternatives,omitempty"`
}

// Index is the queryable view over capabilities.
type Index struct {
	caps  map[string]*Capability
	order []string // insertion order, for deterministic tie-breaking
}

// NewIndex builds an index from implementations, grouping by capability.
//
// An implementation with no contract has no capability and is DROPPED here
// rather than given a synthetic one: it states no guarantee, so there is nothing
// for a query about a guarantee to match.
func NewIndex(impls []Implementation) *Index {
	ix := &Index{caps: map[string]*Capability{}}
	for _, im := range impls {
		key, err := skills.CapabilityKey(im.Skill)
		if err != nil {
			continue
		}
		c, ok := ix.caps[key]
		if !ok {
			c = &Capability{Key: key}
			ix.caps[key] = c
			ix.order = append(ix.order, key)
		}
		c.Implementations = append(c.Implementations, im)
	}
	return ix
}

// Len reports how many distinct capabilities are indexed.
func (ix *Index) Len() int { return len(ix.caps) }

// Get returns one capability by key.
func (ix *Index) Get(key string) (*Capability, bool) {
	c, ok := ix.caps[key]
	return c, ok
}

// Query is a retrieval request.
type Query struct {
	// Text is what the caller actually asked, in their own words.
	Text string
	// Coordinate is the space-time context key; when set, implementations
	// attested here are preferred over ones attested elsewhere.
	Coordinate string
	// Limit caps results (default 5).
	Limit int
}

// field weights. Deliberately close to pkg/kb's, which were tuned on real
// retrieval rather than invented here: a name match is worth several body
// matches, because a name is chosen to be searched for.
const (
	wName      = 4.0
	wDesc      = 3.0
	wPredicate = 3.0
	wEffect    = 1.0
	wExact     = 100.0 // an exact name match outranks everything
)

// Resolve ranks capabilities against a query.
func (ix *Index) Resolve(q Query) []Match {
	terms := tokenize(q.Text)
	limit := q.Limit
	if limit <= 0 {
		limit = 5
	}

	var out []Match
	for _, key := range ix.order {
		c := ix.caps[key]
		m, ok := ix.score(c, q, terms)
		if !ok {
			continue
		}
		out = append(out, m)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Deterministic tie-break: the index must return the same order twice,
		// or nothing built on it is reproducible.
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (ix *Index) score(c *Capability, q Query, terms []string) (Match, bool) {
	primary := elect(c, q.Coordinate)
	var score float64
	var why []string

	lowerQ := strings.ToLower(strings.TrimSpace(q.Text))
	for _, im := range c.Implementations {
		if strings.EqualFold(im.Name, lowerQ) {
			score += wExact
			why = append(why, "exact name match")
			primary = im
			break
		}
	}

	for _, t := range terms {
		for _, im := range c.Implementations {
			if strings.Contains(strings.ToLower(im.Name), t) {
				score += wName
				why = appendOnce(why, "name")
			}
			if strings.Contains(strings.ToLower(im.Description), t) {
				score += wDesc
				why = appendOnce(why, "description")
			}
		}
		// GRAPH EXPANSION: a capability is also described by what it
		// GUARANTEES, so contract predicates and effect atoms are searchable
		// text. This is what lets "verify the build" find a skill whose prose
		// never says "verify" — the contract says `builida`.
		for _, chk := range primary.Skill.Contract {
			if strings.Contains(strings.ToLower(chk.Predicate), t) {
				score += wPredicate
				why = appendOnce(why, "contract")
			}
		}
		for _, e := range primary.Skill.EffectCap {
			if strings.Contains(strings.ToLower(e.String()), t) {
				score += wEffect
				why = appendOnce(why, "effect")
			}
		}
	}

	if score == 0 {
		return Match{}, false
	}
	return Match{
		Capability:   c,
		Key:          c.Key,
		Primary:      primary,
		Name:         primary.Name,
		Score:        score,
		Why:          why,
		Alternatives: len(c.Implementations) - 1,
	}, true
}

// elect picks the implementation to answer with.
//
// Deliberately simple for now: prefer one that can render at the lowest band
// (most bindings = least model needed), then fall back to declaration order.
// Election by ATTESTED success at this coordinate is the real answer and needs
// an evidence floor a single host does not reach alone — acting on a handful of
// runs would discard a sound implementation on noise.
func elect(c *Capability, coordinate string) Implementation {
	if len(c.Implementations) == 0 {
		return Implementation{}
	}
	best := c.Implementations[0]
	bestBound := len(best.Bindings)
	for _, im := range c.Implementations[1:] {
		if len(im.Bindings) > bestBound {
			best, bestBound = im, len(im.Bindings)
		}
	}
	return best
}

// tokenize lowercases and splits on non-letter/digit runs, dropping tokens
// shorter than three characters — below that a token matches everything and
// ranks nothing.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) >= 3 && !stopWords[f] {
			out = append(out, f)
		}
	}
	return out
}

// stopWords are the words a natural-language question is made of. They carry no
// retrieval signal and, left in, they match everything equally — which is worse
// than useless because it flattens the ranking.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "how": true,
	"can": true, "you": true, "that": true, "this": true, "get": true,
	"into": true, "from": true, "want": true, "need": true, "does": true,
	"what": true, "when": true, "where": true, "have": true, "are": true,
}

func appendOnce(dst []string, v string) []string {
	for _, s := range dst {
		if s == v {
			return dst
		}
	}
	return append(dst, v)
}
