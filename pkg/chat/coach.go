package chat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"sync"
	"time"
)

// LIVE AGENT COACHING — P0, the LLM-free auto-coach.
//
// A coach watches a running session's tool.call stream and, when the agent
// starts LOOPING — re-issuing calls without making new distinct progress —
// intervenes to break it out and tell it to deliver. It is the runtime twin of
// the space-time advisor: the advisor explains a FAILED command after the fact;
// the coach steers a doomed loop WHILE it is running.
//
// It is a REPORT CHANNEL, NEVER AN AUTHOR. A coach can press ESC and say a
// sentence; it cannot write a file or merge. That boundary is the whole reason
// it is safe to point one agent at another's live session.
//
// Why ESC and not just a sentence: every agent TUI in this fleet queues a Say
// and reads it only between turns. An agent stuck in a tool loop never reaches
// the between-turns moment, so the sentence sits unread forever. Escape is the
// only thing that reaches it there (foreman proved this live: an agy conductor
// made 224 tool calls, 22 distinct, and only ESC could stop it). So the coach
// interrupts first, THEN speaks.
//
// The signal is deliberately CHEAP and LLM-free: distinct=1 with a climbing
// count is exactly the glm-5.2 / kimi-k3 non-convergence failure the fleet keeps
// measuring. No model call is needed to see it, and none should be spent.

// CoachPolicy configures the auto-coach.
type CoachPolicy struct {
	// RepeatThreshold trips when ONE (tool,input) has been issued this many
	// times. The cheapest non-convergence signal there is.
	RepeatThreshold int
	// RatioThreshold trips when total/distinct reaches this (once MinCalls is
	// met). Catches loops that spread across a handful of calls.
	RatioThreshold float64
	// MinCalls suppresses any trip before this many total calls — do not coach a
	// run that has barely started.
	MinCalls int
	// Cooldown suppresses a re-steer until this many NEW distinct calls have
	// happened since the last one. Sparse by construction: an over-eager coach
	// collapses the worker's own reasoning.
	Cooldown int
	// MaxSteers is a hard cap on interventions per session.
	MaxSteers int
	// Steer is the line injected after the interrupt.
	Steer string
	// Interrupt sends ESC before the Steer. On by default; the only reason to
	// disable it is a probe of whether a plain Say lands (it does not, mid-loop).
	Interrupt bool
	// LogPath, if set, receives one JSON line per steer — the (state -> steer)
	// record that seeds the training loop (P3).
	LogPath string
}

// DefaultCoachPolicy is the P0 "you have the answer, stop" coach.
func DefaultCoachPolicy() CoachPolicy {
	return CoachPolicy{
		RepeatThreshold: 3,
		RatioThreshold:  3.0,
		MinCalls:        3,
		Cooldown:        2,
		MaxSteers:       3,
		Interrupt:       true,
		Steer:           "You appear to be repeating work you have already completed. If you already have the answer, STOP investigating and deliver your final result now.",
	}
}

// SteerRecord is one intervention: the signal that triggered it and what was said.
type SteerRecord struct {
	At       time.Time `json:"at"`
	Reason   string    `json:"reason"`  // "repeat" | "ratio"
	Trigger  string    `json:"trigger"` // the looping call: tool|inputhash
	Count    int       `json:"count"`   // times that call had been issued
	Total    int       `json:"total"`
	Distinct int       `json:"distinct"`
	Repeat   float64   `json:"repeat"`
	Steer    string    `json:"steer"`
	Agent    string    `json:"agent"`
}

// CoachReport summarizes a session after it ends.
type CoachReport struct {
	Total    int           `json:"total_calls"`
	Distinct int           `json:"distinct_calls"`
	Repeat   float64       `json:"repeat_ratio"`
	Steers   []SteerRecord `json:"steers"`
}

// Coach is a live, LLM-free watcher over one Session's tool.call stream.
type Coach struct {
	sess  *Session
	agent string
	pol   CoachPolicy

	mu             sync.Mutex
	counts         map[string]int // (tool|inputhash) -> times seen
	total          int
	distinctAtLast int // distinct count when we last steered
	steers         []SteerRecord
	done           chan struct{}
}

// newCoach builds a coach with no session attached — the form the signal test
// drives directly, feeding it events and asserting the trip decision without
// any live agent or socket IO.
func newCoach(pol CoachPolicy) *Coach {
	return &Coach{pol: pol, counts: map[string]int{}, done: make(chan struct{})}
}

// StartCoach attaches a coach to a running session and begins watching. It
// returns immediately; the coach runs until the context ends. Call Wait to
// block for the watcher to drain after cancelling.
func (s *Session) StartCoach(ctx context.Context, pol CoachPolicy) *Coach {
	c := newCoach(pol)
	c.sess = s
	c.agent = s.Agent
	go c.watch(ctx)
	return c
}

func (c *Coach) watch(ctx context.Context) {
	defer close(c.done)
	path := c.sess.EventsPath()
	if path == "" {
		// No event channel: the coach cannot see tool calls as structured data,
		// so P0 is a no-op here. A silence-based coach is imprecise and belongs to
		// a later phase; refusing to guess is the honest behavior.
		return
	}
	// An INDEPENDENT tail: its own offset, so it never races the session's own
	// eventTail (which WaitIdle drains). Two readers of one append-only file.
	tail := &eventTail{path: path}
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	for {
		if evs, err := tail.drain(); err == nil {
			for _, ev := range evs {
				if ev.Type == EventToolCall {
					c.onToolCall(ev)
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// toolCallData is the payload we key on: same name AND same input is the same
// call. Same tool with different args is progress, not a loop.
type toolCallData struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (c *Coach) onToolCall(ev Event) {
	rec := c.decide(ev)
	if rec == nil {
		return
	}
	// Intervene OUTSIDE the lock — Say/Interrupt do socket IO. ESC first (break
	// the loop), then the sentence (now read, because the loop was broken).
	if c.sess != nil {
		if c.pol.Interrupt {
			_ = c.sess.Interrupt()
			time.Sleep(150 * time.Millisecond) // let the TUI return to its input box
		}
		_ = c.sess.Say(rec.Steer)
	}
	c.logSteer(*rec)
}

// decide records one tool.call, updates the loop counters, and returns a
// SteerRecord when the policy trips (else nil). Pure of any session IO, so the
// signal is testable on its own.
func (c *Coach) decide(ev Event) *SteerRecord {
	var d toolCallData
	_ = json.Unmarshal(ev.Data, &d)
	key := d.Name + "|" + hashInput(d.Input)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[key]++
	c.total++
	count := c.counts[key]
	distinct := len(c.counts)
	ratio := ratioOf(c.total, distinct)

	reason := ""
	switch {
	case c.pol.RepeatThreshold > 0 && count >= c.pol.RepeatThreshold:
		reason = "repeat"
	case c.pol.RatioThreshold > 0 && c.total >= c.pol.MinCalls && ratio >= c.pol.RatioThreshold:
		reason = "ratio"
	}

	trip := reason != "" &&
		c.total >= c.pol.MinCalls &&
		len(c.steers) < c.pol.MaxSteers &&
		(len(c.steers) == 0 || distinct-c.distinctAtLast >= c.pol.Cooldown)
	if !trip {
		return nil
	}
	rec := SteerRecord{
		At: time.Now().UTC(), Reason: reason, Trigger: key, Count: count,
		Total: c.total, Distinct: distinct, Repeat: ratio,
		Steer: c.pol.Steer, Agent: c.agent,
	}
	c.steers = append(c.steers, rec)
	c.distinctAtLast = distinct
	return &rec
}

// Wait blocks until the watcher goroutine has drained after the context ended.
func (c *Coach) Wait() { <-c.done }

// Report summarizes the session so far.
func (c *Coach) Report() CoachReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CoachReport{
		Total:    c.total,
		Distinct: len(c.counts),
		Repeat:   ratioOf(c.total, len(c.counts)),
		Steers:   append([]SteerRecord(nil), c.steers...),
	}
}

func (c *Coach) logSteer(rec SteerRecord) {
	if c.pol.LogPath == "" {
		return
	}
	f, err := os.OpenFile(c.pol.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	if b, err := json.Marshal(rec); err == nil {
		_, _ = f.Write(append(b, '\n'))
	}
}

func hashInput(b []byte) string {
	if len(b) == 0 {
		return "none"
	}
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:6])
}

func ratioOf(total, distinct int) float64 {
	if distinct == 0 {
		return 0
	}
	return math.Round(float64(total)/float64(distinct)*100) / 100
}
