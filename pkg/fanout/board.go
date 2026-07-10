// Package fanout implements bashy's shared-context parallel-agents feature —
// the "blackboard" pattern: N agent instances with slightly different
// instructions work concurrently against ONE evolving shared context (the
// board), each reading a SCOPED view (not the firehose) and posting
// attributed contributions back.
//
// The board is an append-only, provenance-tagged JSON-Lines log — the same
// concurrency-safe substrate graph-contrib and weave's memory use. This file
// is the P0 substrate; fanout.go is the P1 orchestrator; Read's scope filter
// is the P2 context-pollution mitigation.
//
// See dhnt/docs/agentic-design-pattern-gaps.md.
package fanout

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Contribution is one record on the board: the seed, or a post from an agent.
type Contribution struct {
	ID      string   `json:"id"`
	Board   string   `json:"board"`
	Kind    string   `json:"kind"` // seed | post
	By      string   `json:"by,omitempty"`
	At      string   `json:"at"`
	Text    string   `json:"text"`
	Refs    []string `json:"refs,omitempty"` // seed: file/kb references
	Tags    []string `json:"tags,omitempty"`
	Scope   string   `json:"scope,omitempty"` // the angle/lens this belongs to
	Episode string   `json:"episode,omitempty"`
	Forget  bool     `json:"forget,omitempty"`
}

// Board is one shared blackboard, backed by an append-only JSONL log.
type Board struct {
	name string
	path string
}

// Open returns the board named name under root. It does not touch disk until a
// write; a board with no file yet simply has no contributions.
func Open(root, name string) *Board {
	return &Board{name: name, path: filepath.Join(root, name+".jsonl")}
}

func (b *Board) Name() string { return b.name }
func (b *Board) Path() string { return b.path }

// Exists reports whether the board has been created (seeded or posted to).
func (b *Board) Exists() bool {
	_, err := os.Stat(b.path)
	return err == nil
}

// append writes one record as a JSON line. O_APPEND makes concurrent writes
// from parallel instances safe without a lock (records are line-atomic).
func (b *Board) append(c Contribution) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(b.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

// records replays the raw log.
func (b *Board) records() ([]Contribution, error) {
	f, err := os.Open(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Contribution
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var c Contribution
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue // skip a corrupt line rather than lose the whole board
		}
		out = append(out, c)
	}
	return out, sc.Err()
}

// Seed writes the shared seed context. Re-seeding replaces it (same ID).
func (b *Board) Seed(text string, refs []string, by string) error {
	return b.append(Contribution{
		ID: b.name + ":seed", Board: b.name, Kind: "seed",
		By: by, At: now(), Text: text, Refs: refs,
	})
}

// SeedText returns the latest seed text (last writer wins).
func (b *Board) SeedText() (string, string, []string, error) {
	recs, err := b.records()
	if err != nil {
		return "", "", nil, err
	}
	var seed *Contribution
	for i := range recs {
		if recs[i].Kind == "seed" {
			seed = &recs[i]
		}
	}
	if seed == nil {
		return "", "", nil, nil
	}
	return seed.Text, seed.By, seed.Refs, nil
}

// Post appends one contribution. by/scope/tags carry the provenance and the
// lens the post belongs to.
func (b *Board) Post(text, by, scope string, tags []string, episode string) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("fanout: empty contribution")
	}
	return b.append(Contribution{
		ID:    contribID(b.name, by, scope, text),
		Board: b.name, Kind: "post", By: by, At: now(),
		Text: text, Scope: scope, Tags: tags, Episode: episode,
	})
}

// Contributions replays the posts, applying forgets (soft-delete) and
// last-writer-wins per ID. Seeds are excluded.
func (b *Board) Contributions() ([]Contribution, error) {
	recs, err := b.records()
	if err != nil {
		return nil, err
	}
	byID := map[string]Contribution{}
	var order []string
	for _, c := range recs {
		if c.Kind != "post" {
			continue
		}
		if _, seen := byID[c.ID]; !seen {
			order = append(order, c.ID)
		}
		if c.Forget {
			delete(byID, c.ID)
			continue
		}
		byID[c.ID] = c
	}
	out := make([]Contribution, 0, len(order))
	for _, id := range order {
		if c, ok := byID[id]; ok {
			out = append(out, c)
		}
	}
	return out, nil
}

// Status counts live posts per author.
func (b *Board) Status() (map[string]int, int, error) {
	posts, err := b.Contributions()
	if err != nil {
		return nil, 0, err
	}
	byAuthor := map[string]int{}
	for _, c := range posts {
		who := c.By
		if who == "" {
			who = "unknown"
		}
		byAuthor[who]++
	}
	return byAuthor, len(posts), nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// contribID is a stable content hash so re-posting identical content from the
// same author+scope is idempotent (last-writer-wins on replay).
func contribID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])[:16]
}

// relevance is the term-overlap between a post and a lens query (post text +
// tags + scope vs the query). 0 means unrelated.
func relevance(c Contribution, query string) int {
	q := terms(query)
	if len(q) == 0 {
		return 0
	}
	ct := terms(c.Text + " " + strings.Join(c.Tags, " ") + " " + c.Scope)
	n := 0
	for t := range q {
		if ct[t] {
			n++
		}
	}
	return n
}

// sortByRelevance orders posts by descending term-overlap with the query.
// Stable for equal scores (preserves post order). Used by Read's scope filter.
func sortByRelevance(posts []Contribution, query string) {
	sort.SliceStable(posts, func(i, j int) bool {
		return relevance(posts[i], query) > relevance(posts[j], query)
	})
}

// terms is a tiny lowercase word set — deliberately dependency-free so pkg/
// fanout stays a leaf.
func terms(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if len(w) >= 3 {
			out[w] = true
		}
	}
	return out
}
