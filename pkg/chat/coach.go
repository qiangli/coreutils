package chat

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/qiangli/coreutils/pkg/agentpty"
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

	// --- pty-scrape mode (event-less tools: agy, and any third-party CLI) ---
	// When the tool has no event channel, the coach cannot see tool.call as data.
	// It falls back to a GENERIC signal over the terminal output: a loop is
	// "output flowing but distinct content not growing" — the pty analog of the
	// repeat ratio, needing no per-TUI syntax. These tune that detector.
	//
	// PtyWindow is the sliding window of recent significant output lines.
	PtyWindow int
	// PtyNoveltyFloor trips when distinct/window (the novelty ratio) falls to or
	// below this — i.e. the last PtyWindow lines are mostly repeats of each other.
	// Healthy work runs ~0.7–1.0; a churning loop collapses toward 0.
	PtyNoveltyFloor float64
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
		PtyWindow:       40,
		PtyNoveltyFloor: 0.35,
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

// Steerer is the minimal control surface a coach needs to intervene: break the
// current turn (ESC) and inject a line. *Session implements it; so does any run
// with an agentpty control socket (weave), via NewCtlSteerer.
type Steerer interface {
	Interrupt() error
	Say(text string) error
}

var _ Steerer = (*Session)(nil)

// ctlSteerer steers a run straight through its agentpty control socket — the
// weave path, where there is no chat.Session, only a ctlSock.
type ctlSteerer struct{ ctlSock string }

// NewCtlSteerer builds a Steerer over an agentpty control socket. A "" socket
// makes every intervention a no-op (detect-and-log without steering).
func NewCtlSteerer(ctlSock string) Steerer { return ctlSteerer{ctlSock: ctlSock} }

func (s ctlSteerer) Interrupt() error {
	if s.ctlSock == "" {
		return nil
	}
	return agentpty.SendFrame(s.ctlSock, agentpty.VerbatimFrame([]byte{0x1b}))
}

func (s ctlSteerer) Say(text string) error {
	if s.ctlSock == "" {
		return nil
	}
	return agentpty.BrokerSay(s.ctlSock, text)
}

// Coach is a live, LLM-free watcher over an agent run's tool.call stream (event
// mode) or terminal output (pty mode).
type Coach struct {
	sess  *Session
	steer Steerer
	agent string
	pol   CoachPolicy

	mu             sync.Mutex
	counts         map[string]int // (tool|inputhash) -> times seen
	total          int
	distinctAtLast int // distinct count when we last steered
	steers         []SteerRecord
	done           chan struct{}

	// pty-scrape mode state
	ptyOffset  int                 // byte cursor into Session.Output()
	ptyPartial string              // trailing incomplete line carried between polls
	ptyLast    string              // last KEPT normalized line (consecutive-dedup)
	window     []string            // sliding window of significant normalized lines
	winCount   map[string]int      // multiset count for the window
	ptySeen    map[string]struct{} // CUMULATIVE distinct lines, for the report
}

// newCoach builds a coach with no session attached — the form the signal test
// drives directly, feeding it events and asserting the trip decision without
// any live agent or socket IO.
func newCoach(pol CoachPolicy) *Coach {
	return &Coach{pol: pol, counts: map[string]int{}, winCount: map[string]int{}, ptySeen: map[string]struct{}{}, done: make(chan struct{})}
}

// NewLineCoach builds a coach fed one line at a time (its Write method) and
// steering through the given Steerer — the form weave uses: it tees a run's
// decoded output into the coach and lets the pty-novelty detector run over it,
// with no chat.Session involved. Always pty-mode.
func NewLineCoach(pol CoachPolicy, steer Steerer) *Coach {
	c := newCoach(pol)
	c.steer = steer
	return c
}

// Write feeds streamed output to the pty detector, line by line. It is an
// io.Writer so a caller can `io.MultiWriter(log, coach)` a run's output into it.
// Called from the single output-pump goroutine; feedPty locks the shared state.
func (c *Coach) Write(p []byte) (int, error) {
	c.ptyPartial += string(p)
	idx := strings.LastIndexByte(c.ptyPartial, '\n')
	if idx < 0 {
		return len(p), nil // no complete line yet
	}
	complete := c.ptyPartial[:idx]
	c.ptyPartial = c.ptyPartial[idx+1:]
	for _, ln := range strings.Split(complete, "\n") {
		if rec := c.feedPty(ln); rec != nil {
			c.intervene(rec)
		}
	}
	return len(p), nil
}

// StartCoach attaches a coach to a running session and begins watching. It
// returns immediately; the coach runs until the context ends. Call Wait to
// block for the watcher to drain after cancelling.
func (s *Session) StartCoach(ctx context.Context, pol CoachPolicy) *Coach {
	c := newCoach(pol)
	c.sess = s
	c.steer = s
	c.agent = s.Agent
	go c.watch(ctx)
	return c
}

func (c *Coach) watch(ctx context.Context) {
	defer close(c.done)
	if c.sess == nil {
		return
	}
	if c.sess.EventsPath() != "" {
		c.watchEvents(ctx) // precise: structured tool.call stream (ycode)
		return
	}
	c.watchPty(ctx) // generic fallback: loop-from-terminal-output (agy & any pty CLI)
}

// PtyMode reports whether this coach is watching the terminal output (no event
// channel) rather than the structured tool.call stream.
func (c *Coach) PtyMode() bool { return c.sess != nil && c.sess.EventsPath() == "" }

// watchEvents follows the tool's NDJSON event file — the precise path.
func (c *Coach) watchEvents(ctx context.Context) {
	// An INDEPENDENT tail: its own offset, so it never races the session's own
	// eventTail (which WaitIdle drains). Two readers of one append-only file.
	tail := &eventTail{path: c.sess.EventsPath()}
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

// watchPty is the GENERIC, event-less signal. It polls the accumulating terminal
// scrape, normalizes each new line, and feeds the significant ones to a novelty
// detector. No per-TUI syntax: a loop is "output flowing, distinct content not
// growing", which every agent CLI exhibits when it churns.
func (c *Coach) watchPty(ctx context.Context) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		full := c.sess.Output()
		if len(full) > c.ptyOffset {
			chunk := c.ptyPartial + full[c.ptyOffset:]
			c.ptyOffset = len(full)
			lines := strings.Split(chunk, "\n")
			c.ptyPartial = lines[len(lines)-1] // last segment is still being written
			for _, ln := range lines[:len(lines)-1] {
				if rec := c.feedPty(ln); rec != nil {
					c.intervene(rec)
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

// feedPty runs one raw terminal line through the novelty detector and returns a
// SteerRecord when the window has collapsed into a loop (else nil).
func (c *Coach) feedPty(raw string) *SteerRecord {
	norm := normalizeLine(raw)
	if !significant(norm) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if norm == c.ptyLast {
		return nil // an in-place redraw of the same line — not progress, not a loop
	}
	c.ptyLast = norm

	c.ptySeen[norm] = struct{}{} // cumulative, for the report
	c.window = append(c.window, norm)
	c.winCount[norm]++
	c.total++
	w := c.pol.PtyWindow
	if w <= 0 {
		w = 40
	}
	if len(c.window) > w {
		old := c.window[0]
		c.window = c.window[1:]
		if c.winCount[old]--; c.winCount[old] <= 0 {
			delete(c.winCount, old)
		}
	}
	if len(c.window) < w {
		return nil // not enough to judge yet
	}
	distinct := len(c.winCount)
	novelty := float64(distinct) / float64(len(c.window))
	floor := c.pol.PtyNoveltyFloor
	if floor <= 0 {
		floor = 0.35
	}
	if novelty > floor {
		return nil // still making progress
	}
	if len(c.steers) >= c.pol.MaxSteers {
		return nil
	}
	// The offending line is whichever repeats the most in the window.
	trigger, hi := norm, 0
	for k, n := range c.winCount {
		if n > hi {
			trigger, hi = k, n
		}
	}
	rec := SteerRecord{
		At: time.Now().UTC(), Reason: "churn", Trigger: trigger, Count: hi,
		Total: c.total, Distinct: distinct, Repeat: ratioOf(len(c.window), distinct),
		Steer: c.pol.Steer, Agent: c.agent,
	}
	c.steers = append(c.steers, rec)
	// Reset the window so novelty must re-collapse before another steer — the
	// pty-mode cooldown, and it stops one loop from tripping every poll.
	c.window = c.window[:0]
	c.winCount = map[string]int{}
	c.ptyLast = ""
	return &rec
}

// toolCallData is the payload we key on: same name AND same input is the same
// call. Same tool with different args is progress, not a loop.
type toolCallData struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func (c *Coach) onToolCall(ev Event) {
	if rec := c.decide(ev); rec != nil {
		c.intervene(rec)
	}
}

// intervene delivers the steer. ESC first (a queued Say is read only between
// turns, useless to an agent stuck mid-loop), then the sentence (now read,
// because the loop was broken), then the training-log line. Runs OUTSIDE the
// lock — Say/Interrupt do socket IO.
func (c *Coach) intervene(rec *SteerRecord) {
	if c.steer != nil {
		if c.pol.Interrupt {
			_ = c.steer.Interrupt()
			time.Sleep(150 * time.Millisecond) // let the TUI return to its input box
		}
		_ = c.steer.Say(rec.Steer)
	}
	c.logSteer(*rec)
}

var (
	reDigits = regexp.MustCompile(`\d+`)
	reSpace  = regexp.MustCompile(`\s+`)
	reAlnum  = regexp.MustCompile(`[a-zA-Z]`)
)

// normalizeLine turns a raw terminal line into a stable key: strip ANSI/control
// bytes, collapse whitespace, and scrub digit runs to "N" so a spinner's timer
// ("Thought for 5s") and a re-run's counter ("attempt 2") do not read as new
// content each time — which is exactly what would MASK a loop.
func normalizeLine(raw string) string {
	s := SanitizeLine(raw)
	s = reDigits.ReplaceAllString(s, "N")
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// significant drops the noise that must not fill the window: blanks, decoration
// (box-drawing, spinner glyphs, pure punctuation), and lines too short to carry
// an action. Generic — no per-tool knowledge.
func significant(norm string) bool {
	if len(norm) < 8 {
		return false
	}
	return reAlnum.MatchString(norm)
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
	// Distinct is cumulative per mode: event mode counts tool calls, pty mode
	// counts normalized output lines. Only one is populated.
	distinct := len(c.counts)
	if distinct == 0 {
		distinct = len(c.ptySeen)
	}
	return CoachReport{
		Total:    c.total,
		Distinct: distinct,
		Repeat:   ratioOf(c.total, distinct),
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
