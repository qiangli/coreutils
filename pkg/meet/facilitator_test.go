package meet

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// seqRunner replies from a per-agent script, advancing one entry per call, so a
// facilitator can be made to answer differently on successive turns. Past the end
// of a script the last entry repeats — which is what a looping meeting looks like.
type seqRunner struct {
	scripts map[string][]string
	calls   map[string]int
	err     map[string]error
}

func newSeqRunner() *seqRunner {
	return &seqRunner{scripts: map[string][]string{}, calls: map[string]int{}, err: map[string]error{}}
}

func (s *seqRunner) Run(_ context.Context, agent string, _ []string, _ string) (string, int, error) {
	if e := s.err[agent]; e != nil {
		return "", 1, e
	}
	script := s.scripts[agent]
	i := s.calls[agent]
	s.calls[agent]++
	if len(script) == 0 {
		return "contribution from " + agent, 0, nil
	}
	if i >= len(script) {
		i = len(script) - 1
	}
	return script[i], 0, nil
}

func ledger(satisfied, looping, progressing bool, next string) string {
	yn := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}
	return "SATISFIED: " + yn(satisfied) + "\nLOOPING: " + yn(looping) +
		"\nPROGRESSING: " + yn(progressing) + "\nNEXT: " + next +
		"\nINSTRUCTION: address the open point\nREASON: because"
}

func facilitatedSession(t *testing.T) *State {
	t.Helper()
	st := newTestSession(t)
	st.Mode = "facilitated"
	st.MaxTurns = 6
	st.MaxStalls = 3
	_ = st.save()
	return st
}

func TestParseLedger(t *testing.T) {
	l := parseLedger("SATISFIED: no\nLOOPING: YES\nPROGRESSING: no\nNEXT: **codex**\nINSTRUCTION: do x\nREASON: because y")
	if l.Satisfied || !l.Looping || l.Progressing {
		t.Errorf("flags: %+v", l)
	}
	if l.NextSpeaker != "codex" || l.Instruction != "do x" || l.Reason != "because y" {
		t.Errorf("fields: %+v", l)
	}
	if !l.stalling() {
		t.Error("looping + no progress must count as stalling")
	}

	// A garbled reply must never end the meeting nor fake a stall: the backstops
	// bound the loop, not the parse.
	g := parseLedger("I think codex should go next, honestly.")
	if g.Satisfied {
		t.Error("an unparseable reply must not report the request satisfied")
	}
	if g.stalling() {
		t.Error("an unparseable reply must not be read as a stall")
	}
	if g.NextSpeaker != "" {
		t.Errorf("no NEXT line should yield no speaker, got %q", g.NextSpeaker)
	}
}

// Exact match only. Loose matching is what makes `code-reviewer` silently route
// to `reviewer`.
func TestResolveSpeakerIsExact(t *testing.T) {
	roster := []string{"codex", "code-reviewer"}
	for _, in := range []string{"codex", "CODEX", " codex "} {
		if got, ok := resolveSpeaker(in, roster); !ok || got != "codex" {
			t.Errorf("resolveSpeaker(%q) = %q,%v", in, got, ok)
		}
	}
	for _, in := range []string{"", "reviewer", "code", "codex and claude", "claude"} {
		if got, ok := resolveSpeaker(in, roster); ok {
			t.Errorf("resolveSpeaker(%q) should fail, got %q", in, got)
		}
	}
}

// The validation ladder: a facilitator that names a nonexistent participant is
// re-prompted, and a valid retry is accepted.
func TestSelectorLadderRepromptsThenAccepts(t *testing.T) {
	st := facilitatedSession(t)
	r := newSeqRunner()
	r.scripts["claude"] = []string{
		ledger(false, false, true, "reviewer"), // not on the roster
		ledger(false, false, true, "codex"),    // valid
	}
	l := nextLedger(context.Background(), st, st.Participants, "opencode", r)
	if l.Degraded {
		t.Fatalf("a valid retry must not degrade: %+v", l)
	}
	if l.NextSpeaker != "codex" {
		t.Errorf("next = %q, want codex", l.NextSpeaker)
	}
	if r.calls["claude"] != 2 {
		t.Errorf("expected exactly one re-prompt, facilitator called %d times", r.calls["claude"])
	}
}

// After the ladder is exhausted, degrade to a default speaker — and SAY SO. A
// silent fallback looks like a working selector.
func TestSelectorLadderDegradesLoudly(t *testing.T) {
	st := facilitatedSession(t)
	r := newSeqRunner()
	r.scripts["claude"] = []string{ledger(false, false, true, "nobody")}

	l := nextLedger(context.Background(), st, st.Participants, "opencode", r)
	if !l.Degraded {
		t.Fatal("exhausting the ladder must mark the ledger degraded")
	}
	if l.NextSpeaker != "opencode" {
		t.Errorf("next = %q, want the fallback opencode", l.NextSpeaker)
	}
	if !strings.Contains(l.Reason, "failed to name a valid participant") {
		t.Errorf("degradation must explain itself: %q", l.Reason)
	}
	if r.calls["claude"] != maxSelectorAttempts {
		t.Errorf("ladder ran %d times, want %d", r.calls["claude"], maxSelectorAttempts)
	}
	// A dead facilitator degrades too, rather than hanging or routing nowhere.
	st2 := facilitatedSession(t)
	dead := newSeqRunner()
	dead.err["claude"] = errors.New("boom")
	if l2 := nextLedger(context.Background(), st2, st2.Participants, "codex", dead); !l2.Degraded || l2.NextSpeaker != "codex" {
		t.Errorf("dead facilitator should degrade to the fallback, got %+v", l2)
	}
}

// `satisfied` ends the loop, and it is the ONLY outcome the facilitator chooses.
func TestFacilitatedStopsWhenSatisfied(t *testing.T) {
	st := facilitatedSession(t)
	r := newSeqRunner()
	r.scripts["claude"] = []string{
		ledger(false, false, true, "codex"),
		ledger(true, false, true, "codex"), // satisfied on the second ledger
	}
	res, err := runFacilitated(context.Background(), st, r)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Satisfied || res.StoppedBy != "satisfied" {
		t.Fatalf("res = %+v", res)
	}
	if res.Turns != 1 {
		t.Errorf("one participant turn should have run, got %d", res.Turns)
	}
	// Only the selected participant spoke — that is the point of facilitation.
	events, _ := readTranscript(st.ID)
	for _, e := range events {
		if e.Kind == "turn" && e.Speaker != "codex" {
			t.Errorf("unselected participant %s spoke", e.Speaker)
		}
	}
	if countKind(events, "ledger") != 2 {
		t.Errorf("every facilitator decision must be recorded, got %d ledgers", countKind(events, "ledger"))
	}
}

// An agent that never says "satisfied" must not run forever. Termination is the
// orchestrator's, never the model's.
func TestFacilitatedHonorsMaxTurns(t *testing.T) {
	st := facilitatedSession(t)
	st.MaxTurns = 3
	_ = st.save()
	r := newSeqRunner()
	r.scripts["claude"] = []string{ledger(false, false, true, "codex")} // never satisfied

	res, err := runFacilitated(context.Background(), st, r)
	if err != nil {
		t.Fatal(err)
	}
	if res.Turns != 3 || res.StoppedBy != "max_turns" {
		t.Fatalf("res = %+v, want 3 turns stopped by max_turns", res)
	}
	if res.Satisfied {
		t.Error("a backstop stop must not report the request satisfied")
	}
}

// The stall counter: a looping meeting triggers a re-plan rather than calling on
// yet another participant to repeat the loop.
func TestFacilitatedStallTriggersReplanThenGivesUp(t *testing.T) {
	st := facilitatedSession(t)
	st.MaxTurns = 20
	st.MaxStalls = 2
	_ = st.save()
	r := newSeqRunner()
	// Every ledger reports looping + no progress; the replan reply is prose.
	r.scripts["claude"] = []string{ledger(false, true, false, "codex")}

	res, err := runFacilitated(context.Background(), st, r)
	if err != nil {
		t.Fatal(err)
	}
	if res.StoppedBy != "stalled" {
		t.Fatalf("a permanently looping meeting must stop as stalled, got %+v", res)
	}
	if res.Replans < 2 {
		t.Errorf("expected the re-plan escape to fire before giving up, got %d", res.Replans)
	}
	// Critically: it must NOT have burned all 20 turns dispatching participants.
	if res.Turns >= 20 {
		t.Errorf("stall detection must precede dispatch; ran %d turns", res.Turns)
	}
	events, _ := readTranscript(st.ID)
	if countKind(events, "replan") == 0 {
		t.Error("the new approach must be recorded in the transcript")
	}
}

// A cancelled context stops the loop promptly and says so.
func TestFacilitatedHonorsDeadline(t *testing.T) {
	st := facilitatedSession(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := runFacilitated(ctx, st, newSeqRunner())
	if err != nil {
		t.Fatal(err)
	}
	if res.StoppedBy != "deadline" || res.Satisfied {
		t.Fatalf("res = %+v", res)
	}
}

func TestFacilitatedNeedsParticipants(t *testing.T) {
	st := newTestSession(t)
	st.Participants = nil
	if _, err := runFacilitated(context.Background(), st, newSeqRunner()); err == nil {
		t.Fatal("a facilitated meeting with no participants must error, not hang")
	}
}

func TestModeValidation(t *testing.T) {
	if _, err := (&sessionFlags{topic: "t", mode: "swarm"}).newState(); err == nil {
		t.Error("an unknown --mode must be rejected")
	}
	if _, err := (&sessionFlags{topic: "t", mode: "facilitated"}).newState(); err == nil {
		t.Error("--mode facilitated with no participants must be rejected up front")
	}
	st, err := (&sessionFlags{topic: "t", mode: "facilitated", participants: []string{"codex"}}).newState()
	if err != nil {
		t.Fatal(err)
	}
	if !st.facilitated() || st.facilitator() != st.Secretary {
		t.Errorf("facilitator should default to the secretary's agent: %+v", st)
	}
}
