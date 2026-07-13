// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"
)

// Every type in this file is a READ-ONLY PROJECTION of the journal.
//
// None of them is persisted as a source of truth, none of them can be edited, and
// each is a pure function of the entries it was derived from. That is what makes
// them safe: a view cannot drift from the journal, because a view has no state of
// its own to drift with. The moment one of these becomes writable, the system has
// two truths and the older one starts lying.

// WorkstreamState is a strand's lifecycle position — NOT its outcome. A closed
// workstream whose evidence never arrived is closed AND unknown, and collapsing
// those two axes into one is how a board starts reporting wishes as facts.
type WorkstreamState string

const (
	WorkstreamOpen   WorkstreamState = "open"
	WorkstreamClosed WorkstreamState = "closed"
)

// Confidence grades how well a workstream's reported outcome is actually backed.
type Confidence string

const (
	// ConfidenceVerified — the deciding entry carried evidence.
	ConfidenceVerified Confidence = "verified"
	// ConfidenceUnknown — nothing established the outcome one way or the other.
	ConfidenceUnknown Confidence = "unknown"
	// ConfidenceDegraded — an outcome was claimed but could not be established
	// (e.g. success asserted with no evidence).
	ConfidenceDegraded Confidence = "degraded"
)

// Workstream is one strand of work, as the journal describes it.
type Workstream struct {
	Name       string          `json:"name"`
	Title      string          `json:"title,omitempty"`
	State      WorkstreamState `json:"state"`
	Outcome    Outcome         `json:"outcome,omitempty"`
	Confidence Confidence      `json:"confidence"`

	Entries       int `json:"entries"`
	EvidenceCount int `json:"evidence_count"`
	Decisions     int `json:"decisions"`

	OpenedAt    time.Time `json:"opened_at,omitzero"`
	UpdatedAt   time.Time `json:"updated_at,omitzero"`
	LastSummary string    `json:"last_summary,omitempty"`

	// Degraded lists, in the workstream's own words, what could not be established.
	// Kept as prose because the whole value of an unknown is knowing WHICH claim is
	// unproven — a bare count would be a number nobody can act on.
	Degraded []string `json:"degraded,omitempty"`
}

// Board is the derived state of every workstream, plus the journal coordinates it
// was derived from. Watermark + Digest make a board CITABLE: two agents comparing
// boards can tell instantly whether they are looking at the same history.
type Board struct {
	SchemaVersion string       `json:"schema_version"`
	Workstreams   []Workstream `json:"workstreams"`
	Watermark     uint64       `json:"watermark"`
	Digest        string       `json:"digest"`

	// Degraded is true when ANY workstream's outcome could not be established. It
	// is surfaced at the top level so a status check cannot miss it by only reading
	// the happy rows.
	Degraded bool `json:"degraded"`
}

// ProjectBoard derives the board from a sequence of entries.
//
// TRANSCRIPTS ARE SKIPPED ENTIRELY. Nothing about the board may depend on a
// non-authoritative artifact, which is why deleting every transcript on the host
// leaves this function's output bit-identical.
func ProjectBoard(entries []Entry) Board {
	byName := map[string]*Workstream{}
	var order []string

	for _, e := range entries {
		if !e.Kind.Authoritative() {
			continue // transcripts derive nothing, by contract
		}
		name := e.Workstream
		if name == "" {
			continue // seat/reconcile/checkpoint events are host-level, not board rows
		}
		ws, ok := byName[name]
		if !ok {
			ws = &Workstream{Name: name, State: WorkstreamOpen, Confidence: ConfidenceUnknown}
			byName[name] = ws
			order = append(order, name)
		}

		at := parseTime(e.Time)
		if ws.OpenedAt.IsZero() {
			ws.OpenedAt = at
		}
		ws.UpdatedAt = at
		ws.Entries++
		ws.EvidenceCount += len(e.Evidence)
		if e.Summary != "" {
			ws.LastSummary = e.Summary
		}

		switch e.Kind {
		case KindWorkstreamOpen:
			if e.Summary != "" {
				ws.Title = e.Summary
			}
		case KindDecision:
			ws.Decisions++
		case KindWorkstreamClose:
			ws.State = WorkstreamClosed
		}

		// Fold the outcome. The EFFECTIVE outcome is used, never the claimed one:
		// this is the single line that stops an unevidenced "success" from becoming
		// a green row on the board.
		if eff := e.EffectiveOutcome(); eff != "" {
			ws.Outcome = eff
			switch eff {
			case OutcomeUnknown, OutcomeDegraded:
				ws.Confidence = confidenceFor(eff)
				ws.Degraded = append(ws.Degraded, degradedReason(e))
			default:
				if e.HasEvidence() {
					ws.Confidence = ConfidenceVerified
				} else {
					ws.Confidence = ConfidenceUnknown
				}
			}
		}
	}

	sort.Strings(order)
	b := Board{SchemaVersion: SchemaVersion}
	for _, name := range order {
		ws := byName[name]
		if ws.Degraded != nil || ws.Confidence != ConfidenceVerified {
			// A row is only "degraded" for the top-level flag if its CURRENT outcome
			// is unresolved — an early unknown that a later evidenced entry settled is
			// history, not a live problem.
			if ws.Outcome == OutcomeUnknown || ws.Outcome == OutcomeDegraded {
				b.Degraded = true
			}
		}
		b.Workstreams = append(b.Workstreams, *ws)
	}
	if n := len(entries); n > 0 {
		b.Watermark = entries[n-1].Seq
	}
	b.Digest = boardDigest(b.Workstreams)
	return b
}

func confidenceFor(o Outcome) Confidence {
	if o == OutcomeDegraded {
		return ConfidenceDegraded
	}
	return ConfidenceUnknown
}

// degradedReason explains, in one line, why a claim did not land — distinguishing
// "it said it could not tell" from "it claimed success and showed nothing".
func degradedReason(e Entry) string {
	if e.Outcome == OutcomeSuccess && !e.HasEvidence() {
		return "seq " + itoa(e.Seq) + ": claimed success with no evidence — recorded as unknown"
	}
	s := "seq " + itoa(e.Seq) + ": " + string(e.Outcome)
	if e.Summary != "" {
		s += " — " + e.Summary
	}
	return s
}

// boardDigest is a content hash over the board's rows, so a checkpoint's
// reproducibility can be checked by comparing one string.
//
// Deterministic by construction: the rows are sorted by name and marshaled
// canonically, so the same entries always produce the same digest — on any host, in
// any process, in any order the entries happened to be written.
func boardDigest(ws []Workstream) string {
	b, _ := json.Marshal(ws)
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func itoa(u uint64) string {
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}

// Board returns the current board, derived live from the journal.
func (s *Store) Board() (Board, *Replay, error) {
	rep, err := s.Replay()
	if err != nil {
		return Board{}, nil, err
	}
	return ProjectBoard(rep.Entries), rep, nil
}

// Filter selects entries from the log.
type Filter struct {
	Kinds      []Kind
	Workstream string
	Since      time.Time
	Until      time.Time
	Actor      string
	// DegradedOnly keeps only entries whose claim could not be established — the
	// "what do I not actually know?" query, which is the one a successor needs first.
	DegradedOnly bool
	Limit        int
}

// Match reports whether e passes the filter.
func (f Filter) Match(e Entry) bool {
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, e.Kind) {
		return false
	}
	if f.Workstream != "" && e.Workstream != f.Workstream {
		return false
	}
	if f.Actor != "" && !strings.EqualFold(e.Actor.Name, f.Actor) {
		return false
	}
	if f.DegradedOnly && !e.Degraded() {
		return false
	}
	at := parseTime(e.Time)
	if !f.Since.IsZero() && at.Before(f.Since.UTC()) {
		return false
	}
	if !f.Until.IsZero() && at.After(f.Until.UTC()) {
		return false
	}
	return true
}

// Log returns the chronological entries matching a filter. Chronological because
// the journal is append-only: entry order IS time order, and a log that reordered
// them would be inventing a history the chain does not attest to.
//
// Limit keeps the LAST n matches (the recent tail is what a caller wants), and the
// result stays in chronological order.
func (s *Store) Log(f Filter) ([]Entry, *Replay, error) {
	rep, err := s.Replay()
	if err != nil {
		return nil, nil, err
	}
	var out []Entry
	for _, e := range rep.Entries {
		if f.Match(e) {
			out = append(out, e)
		}
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[len(out)-f.Limit:]
	}
	return out, rep, nil
}

// Conversation returns the decision and transcript entries: the record of what was
// decided and, where a transcript survives, how the room got there.
//
// Decisions come first in kind priority but the sequence is preserved — a
// conversation read out of order is not a conversation.
func (s *Store) Conversation(f Filter) ([]Entry, *Replay, error) {
	if len(f.Kinds) == 0 {
		f.Kinds = []Kind{KindDecision, KindTranscript}
	}
	return s.Log(f)
}

// StateChange is one transition in the seat's history: who held it, at what epoch,
// how they got it, and how they lost it.
type StateChange struct {
	Seq       uint64        `json:"seq"`
	At        time.Time     `json:"at"`
	Kind      Kind          `json:"kind"`
	Actor     principalName `json:"actor"`
	Epoch     uint64        `json:"epoch"`
	Summary   string        `json:"summary"`
	Rationale string        `json:"rationale,omitempty"`
	// AuthorizedBy is set on a takeover: the human who sanctioned it.
	AuthorizedBy string `json:"authorized_by,omitempty"`
}

// principalName is the display form of an actor in a history row.
type principalName = string

// History returns the seat's authority history and the checkpoints taken along the
// way — the "how did we get here" view, reconstructed entirely by replay.
func (s *Store) History() ([]StateChange, []CheckpointRef, *Replay, error) {
	rep, err := s.Replay()
	if err != nil {
		return nil, nil, nil, err
	}
	var changes []StateChange
	var cks []CheckpointRef
	for _, e := range rep.Entries {
		switch e.Kind {
		case KindSeatClaimed, KindSeatTakeover, KindSeatReleased:
			changes = append(changes, StateChange{
				Seq: e.Seq, At: parseTime(e.Time), Kind: e.Kind,
				Actor: holderName(e.Actor), Epoch: e.Epoch,
				Summary: e.Summary, Rationale: e.Rationale,
				AuthorizedBy: authorizedByOf(e),
			})
		case KindCheckpoint:
			cks = append(cks, CheckpointRef{
				Seq: e.Seq, At: parseTime(e.Time), ID: e.Ref, Summary: e.Summary,
			})
		}
	}
	return changes, cks, rep, nil
}

// CheckpointRef is a checkpoint as the journal remembers it. The journal's memory
// is what counts: a checkpoint FILE can be deleted and re-derived, but the fact
// that a checkpoint was taken at a watermark is history.
type CheckpointRef struct {
	Seq     uint64    `json:"seq"`
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
	Summary string    `json:"summary,omitempty"`
}
