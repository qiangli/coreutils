package chat

import (
	"encoding/json"
	"testing"
)

// toolCall builds a tool.call event with the given name+input.
func toolCall(name, input string) Event {
	data, _ := json.Marshal(map[string]any{"name": name, "input": json.RawMessage(input)})
	return Event{Type: EventToolCall, Data: data}
}

// feed runs a sequence of calls through decide and returns how many steers fired.
func feed(c *Coach, calls []Event) int {
	n := 0
	for _, ev := range calls {
		if c.decide(ev) != nil {
			n++
		}
	}
	return n
}

func TestCoachTripsOnRepeatedIdenticalCall(t *testing.T) {
	// The glm/kimi-k3 failure: the SAME call, over and over. distinct stays 1.
	pol := DefaultCoachPolicy() // RepeatThreshold 3, MinCalls 3
	c := newCoach(pol)
	same := toolCall("go_test", `{"pkg":"./..."}`)
	steers := feed(c, []Event{same, same, same, same, same})
	if steers != 1 {
		t.Fatalf("want exactly 1 steer (fires at the 3rd identical call, then cooldown holds it), got %d", steers)
	}
	rep := c.Report()
	if rep.Steers[0].Reason != "repeat" {
		t.Errorf("reason = %q, want repeat", rep.Steers[0].Reason)
	}
	if rep.Distinct != 1 || rep.Total != 5 {
		t.Errorf("total/distinct = %d/%d, want 5/1", rep.Total, rep.Distinct)
	}
}

func TestCoachDoesNotTripOnHealthyDistinctWork(t *testing.T) {
	// Every call different: real progress. A coach must never touch this.
	pol := DefaultCoachPolicy()
	c := newCoach(pol)
	var calls []Event
	for i := 0; i < 12; i++ {
		calls = append(calls, toolCall("read_file", `{"path":"f`+string(rune('a'+i))+`.go"}`))
	}
	if n := feed(c, calls); n != 0 {
		t.Fatalf("healthy distinct work must never trip the coach, got %d steers", n)
	}
}

func TestCoachRespectsMaxSteersAndCooldown(t *testing.T) {
	// A run that keeps looping should still be steered only up to MaxSteers, and
	// only after Cooldown distinct calls between interventions.
	pol := DefaultCoachPolicy()
	pol.MaxSteers = 2
	pol.Cooldown = 2
	c := newCoach(pol)
	// First loop on call A -> 1 steer. Then 2 new distinct calls (satisfies
	// cooldown), then loop on B -> 2nd steer. Then loop on C -> capped, no 3rd.
	a := toolCall("t", `{"k":"a"}`)
	b := toolCall("t", `{"k":"b"}`)
	c1 := toolCall("t", `{"k":"c1"}`)
	c2 := toolCall("t", `{"k":"c2"}`)
	d := toolCall("t", `{"k":"d"}`)
	seq := []Event{a, a, a /*steer1*/, c1, c2 /*cooldown met*/, b, b, b /*steer2*/, d, d, d /*capped*/}
	if n := feed(c, seq); n != 2 {
		t.Fatalf("want 2 steers (MaxSteers cap), got %d", n)
	}
}

// feedLines runs raw terminal lines through the pty detector, returns steer count.
func feedLines(c *Coach, lines []string) int {
	n := 0
	for _, ln := range lines {
		if c.feedPty(ln) != nil {
			n++
		}
	}
	return n
}

func TestPtyNormalizeScrubsVolatile(t *testing.T) {
	// A spinner's changing timer/counter must collapse to ONE key, or it reads as
	// new content every frame and masks the loop.
	a := normalizeLine("\x1b[32m▸ Thought for 5s, 386 tokens\x1b[0m")
	b := normalizeLine("▸ Thought for 12s, 1904 tokens")
	if a != b {
		t.Errorf("volatile tokens not scrubbed to a common key:\n  %q\n  %q", a, b)
	}
}

func TestPtyIgnoresRedrawsAndHealthyWork(t *testing.T) {
	c := newCoach(DefaultCoachPolicy())
	var lines []string
	// In-place redraws of the SAME line (a spinner) — consecutive, must collapse.
	for i := 0; i < 30; i++ {
		lines = append(lines, "Working on the task, please wait")
	}
	// Then genuinely distinct progress — every line new.
	for i := 0; i < 60; i++ {
		lines = append(lines, "editing distinct source file number "+string(rune('a'+i%26))+"xyz")
	}
	if n := feedLines(c, lines); n != 0 {
		t.Fatalf("redraws + healthy distinct work must not trip pty coach, got %d steers", n)
	}
}

func TestPtyDetectsChurnLoop(t *testing.T) {
	c := newCoach(DefaultCoachPolicy()) // window 40, novelty floor 0.35
	var lines []string
	// A churn loop: the agent cycles through the SAME 4 actions over and over,
	// interleaved (not consecutive, so dedup won't hide it). 4 distinct / 40 window
	// = novelty 0.10, well below 0.35.
	acts := []string{
		"run_tests on package ./foobar",
		"read_file internal/median/median.go",
		"the tests are still failing here",
		"let me check the implementation again",
	}
	for i := 0; i < 60; i++ {
		lines = append(lines, acts[i%len(acts)])
	}
	if n := feedLines(c, lines); n < 1 {
		t.Fatalf("a churn loop (4 distinct actions cycling) must trip the pty coach, got %d", n)
	}
	if c.Report().Steers[0].Reason != "churn" {
		t.Errorf("reason = %q, want churn", c.Report().Steers[0].Reason)
	}
}

func TestPtyReportHasCumulativeDistinct(t *testing.T) {
	// Regression: the report must show pty-mode distinct (cumulative lines), not
	// the event map — which read 0 and looked like the coach saw nothing.
	c := newCoach(DefaultCoachPolicy())
	for i := 0; i < 20; i++ {
		c.feedPty("distinct progress line number " + string(rune('a'+i)) + "abcdef")
	}
	rep := c.Report()
	if rep.Total != 20 || rep.Distinct != 20 {
		t.Fatalf("pty report total/distinct = %d/%d, want 20/20", rep.Total, rep.Distinct)
	}
}

func TestCoachTripsOnRatioAcrossFewCalls(t *testing.T) {
	// A loop spread across two calls: never 3 of ONE, but ratio climbs.
	pol := CoachPolicy{RepeatThreshold: 99, RatioThreshold: 3.0, MinCalls: 3, MaxSteers: 3, Cooldown: 1}
	c := newCoach(pol)
	a := toolCall("t", `{"k":"a"}`)
	b := toolCall("t", `{"k":"b"}`)
	// a,b,a,b,a,b -> at the 6th call total=6 distinct=2 ratio=3.0 -> trip
	if n := feed(c, []Event{a, b, a, b, a, b}); n < 1 {
		t.Fatalf("ratio loop should trip at least once, got %d", n)
	}
	if c.Report().Steers[0].Reason != "ratio" {
		t.Errorf("reason = %q, want ratio", c.Report().Steers[0].Reason)
	}
}
