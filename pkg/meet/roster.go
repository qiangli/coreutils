package meet

import (
	"fmt"
	"io"
	"sort"

	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/fleet"
)

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
