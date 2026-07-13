// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package steward

import (
	"fmt"
	"os"
	"time"

	"github.com/qiangli/coreutils/pkg/principal"
)

// TTL is how long the seat survives without a heartbeat before its liveness is
// reported as LAPSED.
//
// Thirty minutes, matching the claim registry's lease, and for the same reason: an
// LLM steward works in bursts and may be idle between them. Too short and a
// thinking steward loses the seat mid-thought; too long and a crashed one blocks
// the host until a human intervenes. Erring long is cheap here because the cost is
// a wait, not a deadlock — an expired seat is claimable without --force, and the
// fencing epoch makes a late-returning incumbent safe.
const TTL = 30 * time.Minute

// Liveness is what the heartbeat can honestly tell us. There is deliberately no
// "dead" — see LivenessLapsed.
type Liveness string

const (
	// LivenessLive — the holder heartbeated within the TTL.
	LivenessLive Liveness = "live"
	// LivenessLapsed — the heartbeat is older than the TTL.
	//
	// This proves A LIVENESS LAPSE AND NOTHING MORE. It does not prove the holder
	// crashed, quit, or died: it may be mid-thought, rate-limited, paused at a
	// human prompt, or on a bad network, and it may come back at any moment. The
	// seat is claimable, but the claim FENCES rather than buries the incumbent —
	// which is exactly why a lapse is safe to act on despite proving so little.
	LivenessLapsed Liveness = "lapsed"
	// LivenessUnknown — there is no heartbeat record at all (seat.json is missing
	// or unreadable). Authority still replays from the journal; only liveness is
	// unknown. Reported as unknown rather than silently coerced to "lapsed",
	// because "I have no idea" and "I checked and it is late" are different facts.
	LivenessUnknown Liveness = "unknown"
	// LivenessVacant — nobody holds the seat.
	LivenessVacant Liveness = "vacant"
)

// Authority is the seat state DERIVED FROM THE JOURNAL — the half that matters,
// and the half that survives losing everything else.
//
// Nothing here is read from seat.json. Delete seat.json and every field below is
// reconstructed by replay; that is what makes crash recovery work with no handoff
// note, no goodbye, and no cooperation from the incumbent.
type Authority struct {
	Holder     principal.Ref `json:"holder"`
	Epoch      uint64        `json:"epoch"`
	Vacant     bool          `json:"vacant"`
	AcquiredAt time.Time     `json:"acquired_at,omitzero"`

	// TakenOverFrom / AuthorizedBy / TakeoverReason are set when the current tenure
	// began with a human-authorized takeover, so the record always says who seized
	// the seat, from whom, and on whose authority.
	TakenOverFrom  *principal.Ref `json:"taken_over_from,omitempty"`
	AuthorizedBy   string         `json:"authorized_by,omitempty"`
	TakeoverReason string         `json:"takeover_reason,omitempty"`
}

// Seat is the LIVENESS record (seat.json). It is a CACHE, never an authority:
// holder and epoch are duplicated here only so a status check is one small read
// instead of a full replay, and any disagreement with the journal is resolved in
// the journal's favour, always.
type Seat struct {
	SchemaVersion string        `json:"schema_version"`
	Holder        principal.Ref `json:"holder"`
	Epoch         uint64        `json:"epoch"`
	AcquiredAt    time.Time     `json:"acquired_at"`
	Heartbeat     time.Time     `json:"heartbeat"`
	PID           int           `json:"pid,omitempty"`
	Intent        string        `json:"intent,omitempty"`
}

// View is the complete answer to "who is the steward, and are they alive?" —
// authority from the journal, liveness from the heartbeat, kept visibly separate.
type View struct {
	SchemaVersion string    `json:"schema_version"`
	Authority     Authority `json:"authority"`
	Liveness      Liveness  `json:"liveness"`
	Heartbeat     time.Time `json:"heartbeat,omitzero"`
	Since         time.Time `json:"since,omitzero"`
	PID           int       `json:"pid,omitempty"`
	Intent        string    `json:"intent,omitempty"`

	// Claimable reports whether Claim would succeed right now: the seat is vacant,
	// or its liveness has lapsed/is unknown.
	Claimable bool `json:"claimable"`
}

// deriveAuthority replays the seat lifecycle events. Epoch is MONOTONIC: a
// release does not lower it, because an epoch that could go backwards would let a
// fenced holder become un-fenced by waiting.
func deriveAuthority(rep *Replay) Authority {
	var a Authority
	a.Vacant = true
	for _, e := range rep.Entries {
		switch e.Kind {
		case KindSeatClaimed, KindSeatTakeover:
			a.Holder = e.Actor
			a.Epoch = e.Epoch
			a.Vacant = false
			a.AcquiredAt = parseTime(e.Time)
			a.TakenOverFrom, a.AuthorizedBy, a.TakeoverReason = nil, "", ""
			if e.Kind == KindSeatTakeover {
				if prior := priorHolderOf(e); prior != nil {
					a.TakenOverFrom = prior
				}
				a.AuthorizedBy = authorizedByOf(e)
				a.TakeoverReason = e.Rationale
			}
		case KindSeatReleased:
			a.Vacant = true
			a.Holder = principal.Ref{}
			a.AcquiredAt = time.Time{}
			a.TakenOverFrom, a.AuthorizedBy, a.TakeoverReason = nil, "", ""
			// Epoch is deliberately NOT reset — see the monotonicity note above.
		}
	}
	// The journal's high-water epoch wins over the last seat event's: an entry can
	// never be written under an epoch that a later claim would reuse.
	if rep.MaxEpoch > a.Epoch {
		a.Epoch = rep.MaxEpoch
	}
	return a
}

// priorHolderOf recovers the fenced holder recorded on a takeover entry. It is
// stored in Evidence (kind "principal") so it survives in the hash-chained record
// rather than only in prose.
func priorHolderOf(e Entry) *principal.Ref {
	for _, ev := range e.Evidence {
		if ev.Kind == "principal" && ev.Note == "prior-holder" {
			return &principal.Ref{Name: ev.Ref}
		}
	}
	return nil
}

func authorizedByOf(e Entry) string {
	for _, ev := range e.Evidence {
		if ev.Kind == "human" {
			return ev.Ref
		}
	}
	return ""
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

// SameHolder reports whether two refs are the same logical steward.
//
// Compared by episode-or-(name,host), never by PID: one logical agent runs many
// processes (a shell, a subagent, a hook), and none of them should be told it is
// colliding with itself.
func SameHolder(a, b principal.Ref) bool {
	if a.Episode != "" && a.Episode == b.Episode {
		return true
	}
	return a.Name != "" && a.Name == b.Name && a.Host == b.Host
}

// Self resolves who this process is, minting a stable identity when the ambient
// environment has none. Delegates to the coord identity rule so a steward, a
// claim, and an audit record all name the same agent the same way.
func Self() principal.Ref { return selfRef() }

// ErrHeld is returned by Claim when a LIVE steward already holds the seat. It is
// not an error to route around — it is the singleton working.
type ErrHeld struct {
	View View
}

func (e *ErrHeld) Error() string {
	who := holderName(e.View.Authority.Holder)
	return fmt.Sprintf("steward: the seat is held by %s (epoch %d, last heartbeat %s) — "+
		"this host has exactly one steward. If they are truly gone and you must recover the seat, "+
		"a human authorizes it: `steward takeover --authorized-by <human> --reason <why>`",
		who, e.View.Authority.Epoch, e.View.Heartbeat.Local().Format(time.RFC3339))
}

// ErrFenced is returned when a mutation arrives bearing a superseded epoch — the
// classic returning-zombie case: a steward that lapsed, was taken over, and came
// back believing it still holds the seat.
//
// It is REJECTED, loudly, rather than being allowed to interleave its writes with
// the current steward's. This is the entire payoff of the epoch: a stale heartbeat
// only ever proved a liveness lapse, so the incumbent coming back is not a bug —
// it is expected — and the fence is what makes it harmless.
type ErrFenced struct {
	Presented uint64
	Current   uint64
	Holder    principal.Ref
}

func (e *ErrFenced) Error() string {
	return fmt.Sprintf("steward: fenced — you presented epoch %d but the seat is at epoch %d (held by %s). "+
		"Your tenure ended while you were away; this mutation is rejected. "+
		"Re-read the journal (`steward log`) before doing anything else: the world moved on without you",
		e.Presented, e.Current, holderName(e.Holder))
}

// ErrNotHolder is returned when a non-holder attempts an authoritative write.
type ErrNotHolder struct {
	Actor  principal.Ref
	Holder principal.Ref
	Vacant bool
}

func (e *ErrNotHolder) Error() string {
	if e.Vacant {
		return "steward: the seat is vacant — claim it first (`steward claim`) before recording to the journal"
	}
	return fmt.Sprintf("steward: %s holds the seat; %s may not write to the journal",
		holderName(e.Holder), holderName(e.Actor))
}

// ErrUnauthorized is returned when a takeover arrives without a named human.
type ErrUnauthorized struct{}

func (e *ErrUnauthorized) Error() string {
	return "steward: takeover requires explicit human authorization (--authorized-by <human>). " +
		"Seizing a live seat is a human's call, not an agent's: takeover is the recovery path, " +
		"and an agent that could decide to take over on its own would eventually decide to do it to a healthy steward"
}

func holderName(r principal.Ref) string {
	if r.Name != "" {
		return r.Name
	}
	if r.Episode != "" {
		return r.Episode
	}
	return "an unnamed steward"
}

// Status returns the current seat view: authority replayed from the journal,
// liveness read from the heartbeat file.
func (s *Store) Status(now time.Time) (View, error) {
	now = mustUTC(now)
	rep, err := s.Replay()
	if err != nil {
		return View{}, err
	}
	return s.viewFrom(rep, now)
}

func (s *Store) viewFrom(rep *Replay, now time.Time) (View, error) {
	auth := deriveAuthority(rep)
	v := View{SchemaVersion: SchemaVersion, Authority: auth}

	var seat Seat
	found, err := readJSON(s.seatPath(), &seat)
	if err != nil {
		// A corrupt seat file costs us LIVENESS, not authority. Degrade to unknown
		// rather than failing the whole status: the journal still knows who holds
		// the seat, and that is the question that actually matters.
		found = false
	}

	switch {
	case auth.Vacant:
		v.Liveness = LivenessVacant
		v.Claimable = true
	case !found || seat.Heartbeat.IsZero():
		// Authority survived; the heartbeat did not. Honest answer: unknown.
		v.Liveness = LivenessUnknown
		v.Claimable = true
	default:
		v.Heartbeat = seat.Heartbeat.UTC()
		v.PID = seat.PID
		v.Intent = seat.Intent
		if now.Sub(seat.Heartbeat.UTC()) < TTL {
			v.Liveness = LivenessLive
			v.Claimable = false
		} else {
			v.Liveness = LivenessLapsed
			v.Claimable = true
		}
	}
	v.Since = auth.AcquiredAt
	return v, nil
}

// Claim acquires a VACANT or EXPIRED seat, atomically.
//
// It never negotiates with the incumbent and never requires a handoff note: it
// reads the journal, decides, and writes — all under one lock, so two agents
// racing to claim an empty seat cannot both win.
//
// A change of holder BUMPS THE EPOCH, which fences the previous one. Re-claiming a
// seat you already hold and are live in is an idempotent heartbeat: no new epoch,
// no new journal entry, no churn.
//
// It REFUSES a live seat (ErrHeld). Seizing one is takeover, and takeover is a
// human's decision.
func (s *Store) Claim(holder principal.Ref, intent string, now time.Time) (View, error) {
	now = mustUTC(now)
	var out View
	err := s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		v, err := s.viewFrom(rep, now)
		if err != nil {
			return err
		}

		// Already ours and live: a heartbeat, not a new tenure.
		if !v.Authority.Vacant && SameHolder(v.Authority.Holder, holder) && v.Liveness == LivenessLive {
			if err := s.writeSeat(v.Authority, intent, now); err != nil {
				return err
			}
			out, err = s.viewFrom(rep, now)
			return err
		}
		if !v.Claimable {
			return &ErrHeld{View: v}
		}

		epoch := rep.MaxEpoch + 1
		e := Entry{
			Actor:   holder,
			Epoch:   epoch,
			Kind:    KindSeatClaimed,
			Summary: fmt.Sprintf("%s claimed the steward seat at epoch %d", holderName(holder), epoch),
			Outcome: OutcomeSuccess,
			Evidence: []Evidence{
				{Kind: "seat", Ref: fmt.Sprintf("epoch:%d", epoch), Note: "acquisition"},
			},
		}
		if intent != "" {
			e.Rationale = intent
		}
		if prev := v.Authority; !prev.Vacant {
			// Record whom this claim fenced. The prior holder never agreed to this
			// and was never asked — the record says so plainly.
			e.Evidence = append(e.Evidence, Evidence{
				Kind: "principal", Ref: holderName(prev.Holder), Note: "prior-holder",
			})
			e.Summary += fmt.Sprintf(" (expired seat previously held by %s at epoch %d)",
				holderName(prev.Holder), prev.Epoch)
		}
		if _, err := appendEntry(s.journalPath(), rep, e, now); err != nil {
			return err
		}

		auth := Authority{Holder: holder, Epoch: epoch, AcquiredAt: now}
		if err := s.writeSeat(auth, intent, now); err != nil {
			return err
		}
		out = View{
			SchemaVersion: SchemaVersion, Authority: auth,
			Liveness: LivenessLive, Heartbeat: now, Since: now, PID: os.Getpid(), Intent: intent,
		}
		return nil
	})
	return out, err
}

// Authorization is the human's explicit sanction for seizing a live seat.
type Authorization struct {
	By     string // the human authorizing (required)
	Reason string // why recovery is necessary
}

// Takeover seizes the seat — live or not — under explicit human authorization,
// bumping the epoch so the prior holder is fenced from that instant.
//
// This is the recovery path, and it is deliberately the LOUD one. It does not ask
// the incumbent (an incumbent that could be asked would not need taking over), it
// does not wait for a lease to expire, and it records who authorized it, from
// whom it was taken, and why — because an unexplained seizure of authority is
// indistinguishable from a hijack.
func (s *Store) Takeover(holder principal.Ref, auth Authorization, now time.Time) (View, error) {
	now = mustUTC(now)
	if auth.By == "" {
		return View{}, &ErrUnauthorized{}
	}
	var out View
	err := s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		prev := deriveAuthority(rep)

		epoch := rep.MaxEpoch + 1
		e := Entry{
			Actor:     holder,
			Epoch:     epoch,
			Kind:      KindSeatTakeover,
			Summary:   fmt.Sprintf("%s took over the steward seat at epoch %d, authorized by %s", holderName(holder), epoch, auth.By),
			Rationale: auth.Reason,
			Outcome:   OutcomeSuccess,
			Evidence: []Evidence{
				{Kind: "human", Ref: auth.By, Note: "authorized-by"},
				{Kind: "seat", Ref: fmt.Sprintf("epoch:%d", epoch), Note: "takeover"},
			},
		}
		if !prev.Vacant {
			e.Evidence = append(e.Evidence, Evidence{
				Kind: "principal", Ref: holderName(prev.Holder), Note: "prior-holder",
			})
			e.Summary += fmt.Sprintf(", fencing %s (epoch %d)", holderName(prev.Holder), prev.Epoch)
		}
		if _, err := appendEntry(s.journalPath(), rep, e, now); err != nil {
			return err
		}

		newAuth := Authority{
			Holder: holder, Epoch: epoch, AcquiredAt: now,
			AuthorizedBy: auth.By, TakeoverReason: auth.Reason,
		}
		if !prev.Vacant {
			p := prev.Holder
			newAuth.TakenOverFrom = &p
		}
		if err := s.writeSeat(newAuth, "", now); err != nil {
			return err
		}
		out = View{
			SchemaVersion: SchemaVersion, Authority: newAuth,
			Liveness: LivenessLive, Heartbeat: now, Since: now, PID: os.Getpid(),
		}
		return nil
	})
	return out, err
}

// Release vacates the seat cleanly. It captures NO repository state — no diff, no
// branch, no working tree. A steward hands over a MANDATE, not a checkout; work in
// flight travels by `bashy handoff`, which is a different verb precisely because
// it is a different thing.
//
// Releasing is a courtesy, not a correctness requirement: an unreleased seat still
// expires, and the epoch still fences whoever comes back.
func (s *Store) Release(holder principal.Ref, epoch uint64, note string, now time.Time) error {
	now = mustUTC(now)
	return s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		auth := deriveAuthority(rep)
		if auth.Vacant {
			return nil // releasing a vacant seat is a no-op, not an error
		}
		// Fencing before identity, for the same reason as Record: a returning zombie
		// must be told its tenure ended, not merely that it is a stranger. Releasing
		// on a stale epoch is the most dangerous form of this — a fenced steward
		// "tidying up" on its way out would vacate the seat of the steward that
		// replaced it.
		if epoch != 0 && epoch != auth.Epoch {
			return &ErrFenced{Presented: epoch, Current: auth.Epoch, Holder: auth.Holder}
		}
		if !SameHolder(auth.Holder, holder) {
			return &ErrNotHolder{Actor: holder, Holder: auth.Holder}
		}
		e := Entry{
			Actor:     holder,
			Epoch:     auth.Epoch,
			Kind:      KindSeatReleased,
			Summary:   fmt.Sprintf("%s released the steward seat (epoch %d)", holderName(holder), auth.Epoch),
			Rationale: note,
			Outcome:   OutcomeSuccess,
			Evidence: []Evidence{
				{Kind: "seat", Ref: fmt.Sprintf("epoch:%d", auth.Epoch), Note: "release"},
			},
		}
		if _, err := appendEntry(s.journalPath(), rep, e, now); err != nil {
			return err
		}
		// The seat file is liveness only; with the seat vacated it has nothing left
		// to say. Its removal loses no authority — that lives in the journal.
		if err := os.Remove(s.seatPath()); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	})
}

// Heartbeat refreshes the holder's liveness. It writes no journal entry: a
// heartbeat is not history, it is a pulse, and a journal that recorded every
// pulse would bury the events that matter under them.
func (s *Store) Heartbeat(holder principal.Ref, now time.Time) error {
	now = mustUTC(now)
	return s.withLock(func() error {
		rep, err := s.Replay()
		if err != nil {
			return err
		}
		auth := deriveAuthority(rep)
		if auth.Vacant {
			return &ErrNotHolder{Actor: holder, Vacant: true}
		}
		if !SameHolder(auth.Holder, holder) {
			return &ErrNotHolder{Actor: holder, Holder: auth.Holder}
		}
		return s.writeSeat(auth, "", now)
	})
}

// writeSeat persists the liveness record, preserving the intent and the original
// acquisition time across a refresh so "steward since 3pm" keeps meaning when the
// tenure began rather than when the last command ran.
func (s *Store) writeSeat(auth Authority, intent string, now time.Time) error {
	seat := Seat{
		SchemaVersion: SchemaVersion,
		Holder:        auth.Holder,
		Epoch:         auth.Epoch,
		AcquiredAt:    auth.AcquiredAt,
		Heartbeat:     now,
		PID:           os.Getpid(),
		Intent:        intent,
	}
	var prev Seat
	if found, err := readJSON(s.seatPath(), &prev); err == nil && found {
		if !prev.AcquiredAt.IsZero() && prev.Epoch == auth.Epoch {
			seat.AcquiredAt = prev.AcquiredAt
		}
		if seat.Intent == "" {
			seat.Intent = prev.Intent
		}
	}
	if seat.AcquiredAt.IsZero() {
		seat.AcquiredAt = now
	}
	return writeJSONAtomic(s.seatPath(), seat)
}
