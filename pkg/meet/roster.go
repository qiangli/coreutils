package meet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/secrets"
)

// operableFn is the host-routability gate, indirected so a test can seat agents
// this machine cannot launch. Production always uses capability.Operable — the
// same gate the router and the attendee warnings share.
var operableFn = capability.Operable

// Seating a meeting used to mean naming everyone: --participant this,
// --participant that, and you had to already know which of the fleet were
// worth the tokens. A band makes that one decision instead of N — "everyone
// who can hold a design argument" is `--min-band 3`.

// Seat is an agent admitted to a meeting.
type Seat struct {
	Nick        string
	Agent       string // canonical agent name — what gets recorded
	Binding     string // tool:model
	Band        int
	Reliability string
}

// Skip is an agent the band selected but the host cannot drive.
type Skip struct {
	Agent  string
	Band   int
	Reason string
}

// SeatByBand picks every agent pegged at or above minBand, dropping the ones
// this host cannot actually launch.
//
// The skipped list is returned rather than swallowed. A roster that quietly
// drops an unreachable agent reads, to whoever later opens the minutes, as
// though the whole band was consulted — and a decision credited to a table
// that was never seated is exactly the kind of claim-by-absence-of-evidence
// this fleet is supposed to make impossible.
//
// operable is injected because it is the one part that asks the host a
// question; nil means capability.Operable, the gate the router shares.
func SeatByBand(cat *fleet.Catalog, minBand int, operable func(string) (bool, string)) ([]Seat, []Skip) {
	if operable == nil {
		operable = capability.Operable
	}
	agents, _ := cat.Agents()
	var seats []Seat
	var skips []Skip
	for _, a := range agents {
		_, tool, model, err := cat.Binding(a.Name)
		if err != nil {
			continue // dangling: it has no band, so no band selects it
		}
		if model.Band < minBand {
			continue
		}
		if ok, reason := operable(tool.Name); !ok {
			skips = append(skips, Skip{Agent: a.Name, Band: model.Band, Reason: reason})
			continue
		}
		if ref := tool.CredentialRefFor(model); ref != "" {
			if _, ok := secrets.GrantAgentKey(os.Environ(), ref); !ok {
				skips = append(skips, Skip{
					Agent: a.Name, Band: model.Band,
					Reason: fmt.Sprintf("%s requires the %s provider credential", tool.Name, model.Provider),
				})
				continue
			}
		}
		s := Seat{
			Nick: a.NickName(), Agent: a.Name, Binding: a.MatrixKey(), Band: model.Band,
		}
		if a.Ledger != nil {
			s.Reliability = a.Ledger.Reliability
		}
		seats = append(seats, s)
	}
	// Strongest first, so a roster trimmed from the bottom loses the least.
	sort.SliceStable(seats, func(i, j int) bool {
		if seats[i].Band != seats[j].Band {
			return seats[i].Band > seats[j].Band
		}
		return seats[i].Agent < seats[j].Agent
	})
	return seats, skips
}

// seatByBand fills in sf.participants from the band, and records what it did
// so the caller can print it.
func (sf *sessionFlags) seatByBand() error {
	if sf.minBand == 0 {
		return nil
	}
	if sf.minBand < 1 || sf.minBand > fleet.MaxBand {
		return fmt.Errorf("meet: --min-band %d is out of range (1-%d)", sf.minBand, fleet.MaxBand)
	}
	if len(sf.participants) > 0 {
		return fmt.Errorf("meet: give --min-band or --participant, not both — " +
			"a band seats the table for you, a participant list says you already know who")
	}

	cat := fleet.New()
	total, _ := cat.Agents()
	seats, skips := SeatByBand(cat, sf.minBand, nil)
	if len(seats) == 0 {
		return fmt.Errorf("meet: no operable agent is pegged at band L%d or above — "+
			"`bashy agents list --min-band %d` shows who was considered", sf.minBand, sf.minBand)
	}

	sf.rosterNotes = append(sf.rosterNotes,
		fmt.Sprintf("seating %d of %d agents at band L%d+:", len(seats), len(total), sf.minBand))
	for _, s := range seats {
		sf.participants = append(sf.participants, s.Agent)
		sf.rosterNotes = append(sf.rosterNotes,
			fmt.Sprintf("  %-10s %-22s %-3s %s", s.Nick, s.Binding, fleet.BandLabel(s.Band), s.Reliability))
	}
	for _, k := range skips {
		sf.rosterNotes = append(sf.rosterNotes,
			fmt.Sprintf("skipped: %s (%s) — %s", k.Agent, fleet.BandLabel(k.Band), k.Reason))
	}
	return nil
}

func (sf *sessionFlags) printRoster(w io.Writer) {
	for _, line := range sf.rosterNotes {
		fmt.Fprintln(w, line)
	}
}

// seatLabel shows a seat as it is RECORDED, with the human name beside it.
//
// The two are different on purpose — the record is canonical, the name is
// sayable — and printing only one of them hides half of that. Type `Sable` and
// the preview says `claude-fable5 (Sable)`: you can see both what you asked for
// and what will land in the minutes.
func seatLabel(name string) string {
	a, ok := fleet.New().Agent(name)
	if !ok {
		return name
	}
	if nick := a.NickName(); nick != "" && nick != a.Name {
		return a.Name + " (" + nick + ")"
	}
	return a.Name
}

// canonicalizeRoster rewrites every seat to the canonical agent name before the
// session is saved.
//
// A seat name is not just an argument — it is persisted into the session state
// and stamped onto every Event as its Speaker, so it ends up in the minutes.
// `--participant claude-opus` is a perfectly good thing to TYPE, and a
// catastrophic thing to STORE: `claude-opus` is a floating alias, so the day
// opus4.9 ships, minutes that recorded it silently re-attribute what was said
// to a model that never said it. Same for a nickname, which an operator can
// reassign with `agents set --nick`.
//
// So the seat is resolved once, here, and what gets written down is the agent's
// canonical name. Speak the alias; record the address.
//
// A name that resolves to no agent — a bare tool like `claude`, or a harness
// with no binding yet — is left exactly as typed. It is not an alias for
// anything, so there is nothing to canonicalize and nothing to rot.
func (sf *sessionFlags) canonicalizeRoster() {
	for i, p := range sf.participants {
		sf.participants[i] = canonAgent(p)
	}
	sf.secretary = canonAgent(sf.secretary)
	sf.chair = canonAgent(sf.chair)
}

// routableRoster refuses a roster containing a seat this host cannot drive.
//
// routableSeat existed and was reachable from exactly ONE caller — inviteTo. So
// `meet invite` was gated and `meet start --participant` was not, and anything
// at all could be seated at creation time. That is not hypothetical: a live
// conductor room on this host was created with `root-supervisor` as its sole
// participant, a name the fleet does not know and nothing can drive. The room
// reported `open` forever at round 0 with "no contribution", and a human message
// sent into it was accepted and read by nobody.
//
// Which is precisely what routableSeat's own comment predicts: seating a name
// that is not an agent "would mean every round from then on records a failed
// turn for a participant that was never real". The gate was right; it was simply
// not on the path that creates meetings.
//
// Creation only. Validate is deliberately left alone: it runs when an existing
// session is LOADED, and refusing to load the rooms that already carry an
// unroutable seat would turn a bad roster into an unreadable meeting —
// destroying the record instead of preventing the next one.
//
// The human is not checked. `Human` is its own field on State and is a person,
// not something this host drives.
func (sf *sessionFlags) routableRoster() error {
	for _, p := range sf.participants {
		if err := routableSeat(p); err != nil {
			return err
		}
	}
	// Secretary and chair are agent seats too when set — Validate already
	// forbids either from doubling as a participant — and a secretary that
	// cannot be driven keeps no minutes.
	for _, seat := range []string{sf.secretary, sf.chair} {
		if strings.TrimSpace(seat) == "" {
			continue
		}
		if err := routableSeat(seat); err != nil {
			return err
		}
	}
	return nil
}

// canonAgent resolves any name a human might type — a nickname, a family alias,
// a tool:model binding — to the canonical agent name. A name that belongs to no
// agent is returned untouched: it is an alias for nothing, so there is nothing
// to resolve.
//
// Everything that seats, records, or FILTERS BY a seat goes through here, so
// they all agree on what a name means. An observer filtering on `Sable` must
// match turns recorded as `claude-fable5`, or it watches a blank screen and
// concludes nobody spoke.
func canonAgent(name string) string {
	if a, ok := fleet.New().Agent(name); ok {
		return a.Name
	}
	return name
}

// --- the mutable roster ------------------------------------------------------
//
// A room is a conversation, and a conversation gains and loses people while it is
// running. The roster is therefore not fixed at `start`: the organizer invites a
// second agent mid-thread and it joins the next round, with no new session and no
// re-seating of the ones already talking.
//
// Two rules hold it together. Every mutation goes through Validate(), so a seat
// added at minute forty is held to exactly the invariants a seat named at minute
// zero was — the roster cannot decay into a state `start` would have refused. And
// every mutation is RECORDED as a transcript event: a roster that changed
// silently would leave the minutes attributing a round to a table that was never
// seated that way, which is the same claim-by-absence-of-evidence the band-skip
// list exists to prevent.

// ErrNotOrganizer is returned when someone other than the organizer tries to
// change the roster or close the room. Any member may post; only the organizer
// convenes and dissolves.
//
// It is a sentinel, and wrapped rather than replaced by the callers below, so a
// transport can classify it (the HTTP layer answers 403) instead of matching on
// prose that will be reworded.
var ErrNotOrganizer = errors.New("meet: only the organizer may change the roster")

// requireOrganizer enforces the organizer privilege for a roster change.
//
// An EMPTY initiator is the unnamed agent that called `meet consult` — there is
// nobody to check the actor against, and refusing everybody would freeze that
// room's roster permanently with no way to recover it. So the check does not
// apply rather than failing closed; a room that wants the privilege names its
// organizer, which `meet start` always does.
func requireOrganizer(st *State, actor string) error {
	org := st.initiatorName()
	if org == "" {
		return nil
	}
	who := strings.TrimSpace(actor)
	// Compared canonically as well as literally: the organizer may have been
	// named by nickname at `start` and be addressed by its canonical name here.
	if strings.EqualFold(who, org) || (who != "" && strings.EqualFold(canonAgent(who), canonAgent(org))) {
		return nil
	}
	// Permanent role rooms have a second, deliberately narrow organizer: the
	// verified current role holder recorded by the role lifecycle. This grants
	// roster management, not closure (permanent rooms cannot be closed at all).
	for _, holder := range st.RoleHolders {
		if strings.EqualFold(who, holder) || (who != "" && strings.EqualFold(canonAgent(who), canonAgent(holder))) {
			return nil
		}
	}
	if who == "" {
		who = "an unnamed caller"
	}
	return fmt.Errorf("meet: %s cannot change the roster of %s — %s convened it. "+
		"Any member may post; only the organizer or current permanent-role holder invites or removes: %w",
		who, st.ID, org, ErrNotOrganizer)
}

// routableSeat refuses a name nothing on this host can be addressed as.
//
// The gate is deliberately the WEAKER of two questions, because they fail in
// opposite directions. A name the fleet knows is routable by definition, whether
// or not its binary is installed today — that is an operability warning
// (attendeeWarnings), not a reason to refuse a seat, and a laptop with codex
// uninstalled must still be able to build a roster for a host that has it. A name
// the fleet does NOT know is only meaningful if it is a tool that exists here.
// Everything else — a typo, a human's name, a channel — is not an agent at all,
// and seating it would mean every round from then on records a failed turn for a
// participant that was never real.
func routableSeat(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("meet: no agent named")
	}
	if _, ok := fleet.New().Agent(name); ok {
		return nil
	}
	if ok, _ := operableFn(capability.ResolveTool(name)); ok {
		return nil
	}
	return fmt.Errorf("meet: %q is not an agent this host can drive — "+
		"it is not in the fleet (`bashy agents list`) and there is no such tool on PATH", name)
}

// Invite seats an agent in a running room. Organizer-only.
//
// It is IDEMPOTENT: inviting someone already at the table is a no-op, not a
// second seat. A duplicate seat dilutes a vote while looking like diversity, and
// a caller retrying after a dropped response must not be the way one appears.
func Invite(ref, actor, agent string) error {
	st, err := loadMeeting(ref)
	if err != nil {
		return err
	}
	return inviteTo(st, actor, agent)
}

// inviteTo is Invite against an already-loaded session, so an in-process caller
// (the REPL) mutates the same State it is about to run a round with, rather than
// a second copy of it that its own next save would clobber.
func inviteTo(st *State, actor, agent string) error {
	if err := requireOrganizer(st, actor); err != nil {
		return err
	}
	if err := routableSeat(agent); err != nil {
		return err
	}
	if err := ensureRoomSecretary(context.Background(), st); err != nil {
		return err
	}
	// Canonicalized before it is stored, for the reason canonicalizeRoster gives:
	// the seat name is stamped onto every Event this agent produces, and an alias
	// re-attributes those turns the day it floats to another model.
	name := canonAgent(strings.TrimSpace(agent))
	for _, p := range st.Participants {
		if strings.EqualFold(p, name) {
			return nil // already seated
		}
	}

	prev := st.Participants
	st.Participants = append(append([]string(nil), prev...), name)
	// The full role invariants, not a subset: a seat added mid-meeting must not
	// be able to make the secretary a participant or duplicate the chair.
	if err := st.Validate(); err != nil {
		st.Participants = prev
		return err
	}
	if err := st.save(); err != nil {
		st.Participants = prev
		return err
	}
	_, err := record(st, "invite", actorLabel(st, actor), "", fmt.Sprintf("invited %s", seatLabel(name)))
	return err
}

// Kick removes an agent from a running room. Organizer-only.
func Kick(ref, actor, agent string) error {
	st, err := loadMeeting(ref)
	if err != nil {
		return err
	}
	return kickFrom(st, actor, agent)
}

// kickFrom is Kick against an already-loaded session — see inviteTo.
//
// Unlike Invite this is NOT a silent no-op when the name is absent. Inviting
// twice and inviting once are the same intent; removing somebody who was never
// there means the caller is looking at a roster that does not exist, and saying
// so is cheaper than letting them believe they trimmed the table.
func kickFrom(st *State, actor, agent string) error {
	if err := requireOrganizer(st, actor); err != nil {
		return err
	}
	name := canonAgent(strings.TrimSpace(agent))
	kept := make([]string, 0, len(st.Participants))
	found := ""
	for _, p := range st.Participants {
		if found == "" && strings.EqualFold(p, name) {
			found = p
			continue
		}
		kept = append(kept, p)
	}
	if found == "" {
		return fmt.Errorf("meet: %s is not seated in %s — participants: %s",
			agent, st.ID, strings.Join(st.Participants, ", "))
	}

	prev := st.Participants
	st.Participants = kept
	// A chair with nobody left to call on is the invariant this can violate.
	if err := st.Validate(); err != nil {
		st.Participants = prev
		return err
	}
	if err := st.save(); err != nil {
		st.Participants = prev
		return err
	}
	_, err := record(st, "kick", actorLabel(st, actor), "", fmt.Sprintf("removed %s", seatLabel(found)))
	return err
}

// actorLabel names the speaker of a roster event. An unnamed caller is recorded
// as such rather than as the room's human, who did not do it.
func actorLabel(st *State, actor string) string {
	if a := strings.TrimSpace(actor); a != "" {
		return a
	}
	if org := st.initiatorName(); org != "" {
		return org
	}
	return "an unnamed caller"
}

// toolOf resolves a seat to the tool behind it, so the registry can be asked what
// that tool needs. A bare tool name is already the answer.
func toolOf(seat string) string {
	if a, ok := fleet.New().Agent(seat); ok {
		return a.Tool
	}
	return seat
}
