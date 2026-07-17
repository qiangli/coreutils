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
