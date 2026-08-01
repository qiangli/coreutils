package kb

import (
	"bufio"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Use history — the data a memory needs in order to decay.
//
// Nothing in this store previously recorded that a page had been USED. Pages
// carry provenance (who wrote it, when, from what evidence) and a status
// ladder, but no use count and no last-used time, so there was no way to rank
// by recency or frequency, no way to retire what nobody reads, and no way to
// measure whether the store gets better or worse as it grows. That last one is
// the reason this exists: the growth axis (average accuracy, backward transfer
// i.e. forgetting, forward transfer) is a task×time measurement, and it cannot
// be computed at all without a time series of use.
//
// Two design decisions, both load-bearing:
//
//  1. **Use history lives in an append-only LOG, never in the page.** Writing a
//     counter into a page on every read would rewrite the file on a read path,
//     churn the git history of the committed repo-scoped store, and turn a
//     lock-free reader into a writer. It would also put a clock-derived value
//     inside the record, which is the trap the skill graph's KeyProbes ratchet
//     already exists to prevent. Activation is DERIVED, on demand, by replaying
//     reads.jsonl.
//
//  2. **An OPEN is the signal, not an impression.** The obvious thing to record
//     is "this page appeared in a result list", and it is wrong: the ranker
//     would then be scoring its own past output, and every early mistake would
//     compound. What is recorded instead is what the CALLER did — opening a
//     page (`kb show`) and the mutations already in journal.jsonl (validate,
//     update, supersede). Those are the reader's judgement, not the ranker's,
//     so the loop is not closed on itself.

// useRecord is one appended line in reads.jsonl.
type useRecord struct {
	Op      string    `json:"op"` // open
	Slug    string    `json:"slug"`
	Tool    string    `json:"tool,omitempty"`
	Episode string    `json:"episode,omitempty"`
	At      time.Time `json:"at"`
}

func (s *Store) readsPath() string { return filepath.Join(s.dir, "reads.jsonl") }

// RecordOpen appends an open. Best-effort and silent on failure, exactly like
// the journal: reading a page is the operation, the log is the trail, and a
// full disk must never make `kb show` fail.
func (s *Store) RecordOpen(slug string) {
	rec := useRecord{Op: "open", Slug: slug, Tool: ToolID(), Episode: EpisodeID(), At: time.Now().UTC()}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(s.readsPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// Use is the derived use history of one page.
type Use struct {
	N     int         // how many times it was used
	Last  time.Time   // most recent use
	Times []time.Time // every use, oldest first — the input to base-level activation
}

// BaseLevel is ACT-R's base-level activation: B = ln Σ (now − t_k)^(−d).
//
// It is the standard model of how retrievability falls with disuse and rises
// with repetition, and it is arithmetic — no model, no training. d is the decay
// rate; 0.5 is the conventional default. A page used once long ago and a page
// used ten times this week are separated by this term and by nothing else in
// the ranker.
//
// Returns 0 (not −Inf) for an unused page so the term is neutral rather than
// disqualifying: never having been opened is not evidence of irrelevance,
// which is the same absence-of-evidence rule the rest of this stack obeys.
func (u Use) BaseLevel(now time.Time, d float64) float64 {
	if len(u.Times) == 0 {
		return 0
	}
	var sum float64
	for _, t := range u.Times {
		age := now.Sub(t).Hours()
		if age < 1.0/60 { // floor at one minute: t^-d explodes at t→0
			age = 1.0 / 60
		}
		sum += math.Pow(age, -d)
	}
	if sum <= 0 {
		return 0
	}
	return math.Log(sum)
}

// DefaultDecay is ACT-R's conventional d.
const DefaultDecay = 0.5

// UseHistory replays reads.jsonl plus the mutation journal into per-slug use
// history. Both logs are append-only and tolerant: a corrupt line is skipped
// rather than failing the read, mirroring the page-list and journal-replay
// behaviour.
//
// Mutations count as uses because validating, updating or superseding a page
// is a stronger signal of relevance than opening it — and they are already
// being recorded, so the history starts non-empty on any store with a history.
func (s *Store) UseHistory() map[string]*Use {
	out := map[string]*Use{}
	add := func(slug string, at time.Time) {
		if slug == "" {
			return
		}
		u := out[slug]
		if u == nil {
			u = &Use{}
			out[slug] = u
		}
		u.N++
		u.Times = append(u.Times, at)
		if at.After(u.Last) {
			u.Last = at
		}
	}
	scan := func(path string, fn func(line []byte)) {
		f, err := os.Open(path)
		if err != nil {
			return
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" {
				fn([]byte(line))
			}
		}
	}
	scan(s.readsPath(), func(line []byte) {
		var r useRecord
		if json.Unmarshal(line, &r) == nil {
			add(r.Slug, r.At)
		}
	})
	scan(s.journalPath(), func(line []byte) {
		var r journalRecord
		if json.Unmarshal(line, &r) != nil {
			return
		}
		switch r.Op {
		case "validate", "update", "supersede":
			add(r.Slug, r.At)
		}
	})
	return out
}
