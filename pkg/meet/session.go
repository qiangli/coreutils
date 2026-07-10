// Package meet implements `bashy meet` — a multi-participant deliberation
// session where agentic CLIs and a human take turns, and a dedicated
// notes-only secretary keeps the minutes and files them.
//
// P0 (this cut) is deliberately minimal: local sessions only, sequential
// round-robin turns, a notes-only secretary, and a docs/ filer. See
// dhnt/docs/bashy-meet.md for the full design and the deferred rungs.
package meet

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const schemaVersion = "bashy-meet-v1"

// nowFn is indirected so tests get deterministic timestamps.
var nowFn = time.Now

// Turn outcome statuses. Only ok and abstain are successes: an ABSTAIN is a
// deliberate "no comment" on an optional open question, which is a valid
// contribution, not a tool failure. Everything else nets down the participant's
// operability score and is reported per-participant in the minutes.
const (
	statusOK      = "ok"
	statusEmpty   = "empty"   // agent ran, produced nothing
	statusTimeout = "timeout" // exceeded --turn-timeout
	statusError   = "error"   // non-zero exit / launch failure
	statusShort   = "short"   // below --min-turn-chars
	statusAbstain = "abstain" // optional question, explicitly no comment
	statusInvalid = "invalid" // poll answer outside the choice set
)

// Event is one append-only entry in a meeting transcript.
type Event struct {
	Round   int       `json:"round"`
	Speaker string    `json:"speaker"`
	Role    string    `json:"role,omitempty"`
	Kind    string    `json:"kind"` // agenda|human|turn|decision|action|poll|vote|question|confirm|context
	Text    string    `json:"text"`
	File    string    `json:"file,omitempty"` // per-turn full-text file (context-offloading target)
	TS      time.Time `json:"ts"`

	// Turn outcome, recorded so a reader can tell a timeout from an empty reply
	// from a crash without re-reading logs. Absent on legacy events — statusOf()
	// reconstructs it from the marker text.
	Status   string `json:"status,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
	Chars    int    `json:"chars,omitempty"`
	DurMS    int64  `json:"duration_ms,omitempty"`

	// Poll / open-question fields.
	Question string   `json:"question,omitempty"`
	Choice   string   `json:"choice,omitempty"`  // on a vote: the normalized answer
	Choices  []string `json:"choices,omitempty"` // on a poll: the permitted answers
}

// statusOf reports an event's outcome, reconstructing it for transcripts written
// before Status existed so `meet show` works on old sessions.
func statusOf(e Event) string {
	if e.Status != "" {
		return e.Status
	}
	switch {
	case strings.Contains(e.Text, "timed out"):
		return statusTimeout
	case strings.Contains(e.Text, "returned no content"):
		return statusEmpty
	case strings.Contains(e.Text, "unavailable this turn"):
		return statusError
	}
	return statusOK
}

// contributed reports whether an event carries a real contribution (so an
// abstention counts as coverage, but a crash does not).
func contributed(e Event) bool {
	s := statusOf(e)
	return s == statusOK || s == statusAbstain
}

// redactHome rewrites the user's home directory to `~` anywhere it appears.
// Applied to every string that reaches the published minutes: agent CLIs print
// their workdir in startup banners, and the minutes are committed to a repo that
// may be public. Without this, `/Users/<name>/…` leaks on every meeting.
func redactHome(s string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return s
	}
	if home = strings.TrimRight(home, string(os.PathSeparator)); len(home) < 2 {
		return s
	}
	return strings.ReplaceAll(s, home, "~")
}

// writeTurnFile persists an event's full text under the session's turns/ dir and
// returns the absolute path. Context offloading (LangChain Deep Agents pattern):
// the transcript passed to attendees carries a head/tail PREVIEW + a file link,
// and the full bytes live here for read-on-demand. Best-effort; "" on failure.
func writeTurnFile(id string, e Event) string {
	dir, err := storeDir(id)
	if err != nil {
		return ""
	}
	turns := filepath.Join(dir, "turns")
	if err := os.MkdirAll(turns, 0o755); err != nil {
		return ""
	}
	sum := sha256.Sum256([]byte(e.Text))
	name := fmt.Sprintf("%03d-%s-%s-%s.txt", e.Round, e.Kind, slugify(e.Speaker), hex.EncodeToString(sum[:])[:6])
	path := filepath.Join(turns, name)
	if err := os.WriteFile(path, []byte(e.Text), 0o644); err != nil {
		return ""
	}
	return path
}

// State is the durable meeting header, saved as state.json.
type State struct {
	Schema       string    `json:"schema"`
	ID           string    `json:"id"`
	Topic        string    `json:"topic"`
	Agenda       []string  `json:"agenda,omitempty"`
	Secretary    string    `json:"secretary"`
	Participants []string  `json:"participants"`
	Human        string    `json:"human"`
	Mode         string    `json:"mode"`
	Status       string    `json:"status"`
	Cwd          string    `json:"cwd"`
	Out          string    `json:"out,omitempty"`
	TurnTimeout  string    `json:"turn_timeout,omitempty"` // per-turn agent timeout, e.g. "20m"
	Created      time.Time `json:"created"`
	Round        int       `json:"round"`

	// Initiator is who convened the meeting and therefore who must confirm it
	// may conclude. Kind is "human" or "agent" — an agent-initiated meeting is
	// confirmed by asking that agent, so `meet` works as a tool call.
	Initiator     string `json:"initiator,omitempty"`
	InitiatorKind string `json:"initiator_kind,omitempty"`

	// DecisionMode is "infer" (default — the secretary may record a decision the
	// meeting converged on, tagged as inferred) or "explicit" (only decisions a
	// participant stated outright).
	DecisionMode string `json:"decision_mode,omitempty"`

	// MinTurnChars, when > 0, marks a reply shorter than this as `short` — a
	// participant that answers "ok" did not really attend.
	MinTurnChars int `json:"min_turn_chars,omitempty"`

	// Context is the shared source set every participant reads before its first
	// turn, so the panel reviews the same files rather than guessing.
	Context []string `json:"context,omitempty"`
}

func (s *State) initiatorName() string {
	if n := strings.TrimSpace(s.Initiator); n != "" {
		return n
	}
	return s.Human
}

func (s *State) initiatorKind() string {
	if k := strings.TrimSpace(s.InitiatorKind); k == "agent" {
		return "agent"
	}
	return "human"
}

// initiatorLabel renders the initiator for humans. An agent that convened a
// meeting without naming itself is just "an agent"; repeating "agent (agent)"
// tells the reader nothing.
func (s *State) initiatorLabel() string {
	name, kind := s.initiatorName(), s.initiatorKind()
	if strings.EqualFold(name, kind) {
		return "an unnamed " + kind + " (pass --initiator to name it)"
	}
	return fmt.Sprintf("%s (%s)", name, kind)
}

func (s *State) decisionMode() string {
	if strings.EqualFold(strings.TrimSpace(s.DecisionMode), "explicit") {
		return "explicit"
	}
	return "infer"
}

// Decision is one recorded decision. Inferred marks a decision the secretary
// read out of the meeting's consensus rather than one a participant stated
// outright — the reader must be able to tell them apart.
//
// Support names the participants the secretary says agreed. It is the grounding
// contract: dialogue summarizers invent decisions at a measured ~23% rate, and
// the dominant error class is "circumstantial inference" — a decision that was
// implied but never made. An INFERRED decision therefore requires a proposal AND
// an acceptance (>= 2 named supporters); one that cannot name them is demoted to
// an open question by demoteUnsupported, in code, not by asking the LLM nicely.
type Decision struct {
	Text     string   `json:"text"`
	Inferred bool     `json:"inferred,omitempty"`
	Support  []string `json:"support,omitempty"`
}

// minInferredSupport is the acceptance threshold: a proposer plus at least one
// participant who agreed.
const minInferredSupport = 2

// demoteUnsupported moves every inferred decision that cannot name enough
// supporters into the open-questions list. Discussion of an option is not a
// decision, and the secretary does not get to blur that.
func (s *Synthesis) demoteUnsupported() {
	kept := s.Decisions[:0]
	for _, d := range s.Decisions {
		if d.Inferred && len(d.Support) < minInferredSupport {
			s.OpenQuestions = append(s.OpenQuestions,
				fmt.Sprintf("%s (raised, but no recorded agreement — not a decision)", d.Text))
			continue
		}
		kept = append(kept, d)
	}
	s.Decisions = kept
}

// Synthesis is the secretary's derived view of a meeting: decisions, actions,
// risks, open questions, corrections, and a summary.
//
// It lives in its own file, NOT in the append-only transcript, and the latest
// pass wins. That is what makes `meet amend` idempotent: re-running the
// secretary rewrites this file instead of appending a second set of markers.
// Human `/decision` and `/action` markers stay in the transcript and remain
// authoritative — the secretary's job is to extract, never to overrule.
type Synthesis struct {
	Schema        string     `json:"schema"`
	By            string     `json:"by"`
	At            time.Time  `json:"at"`
	Mode          string     `json:"mode"` // infer|explicit
	Decisions     []Decision `json:"decisions,omitempty"`
	Actions       []string   `json:"actions,omitempty"`
	Risks         []string   `json:"risks,omitempty"`
	OpenQuestions []string   `json:"open_questions,omitempty"`
	Corrections   []string   `json:"corrections,omitempty"`
	Summary       string     `json:"summary,omitempty"`
}

func (s *Synthesis) save(id string) error {
	dir, err := storeDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.Schema = schemaVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "synthesis.json"), b)
}

// loadSynthesis returns the last secretary pass, or nil when none has run.
func loadSynthesis(id string) *Synthesis {
	dir, err := storeDir(id)
	if err != nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, "synthesis.json"))
	if err != nil {
		return nil
	}
	var s Synthesis
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return &s
}

// baseDir is the root of the local session store. Overridable via
// BASHY_MEET_DIR (used by tests and by operators who want a custom location).
func baseDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_MEET_DIR")); d != "" {
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bashy", "meet"), nil
}

func storeDir(id string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, id), nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 48 {
		out = strings.Trim(out[:48], "-")
	}
	if out == "" {
		out = "meeting"
	}
	return out
}

// newID derives a stable session id from the topic + timestamp.
func newID(topic string, now time.Time) string {
	sum := sha256.Sum256([]byte(topic + now.Format(time.RFC3339Nano)))
	short := hex.EncodeToString(sum[:])[:4]
	return fmt.Sprintf("%s-%s-%s", now.Format("2006-01-02"), slugify(topic), short)
}

func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *State) save() error {
	dir, err := storeDir(s.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.Schema = schemaVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "state.json"), b)
}

func loadState(id string) (*State, error) {
	dir, err := storeDir(id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "state.json"))
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func appendEvent(id string, e Event) error {
	dir, err := storeDir(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// O_APPEND makes concurrent turn-writes safe without a lock.
	f, err := os.OpenFile(filepath.Join(dir, "transcript.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

func readTranscript(id string) ([]Event, error) {
	dir, err := storeDir(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(filepath.Join(dir, "transcript.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func listSessions() ([]*State, error) {
	base, err := baseDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []*State
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if s, err := loadState(e.Name()); err == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
