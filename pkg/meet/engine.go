package meet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/chat"
)

const turnGuard = "You are a participant in a planning meeting. Read the topic, agenda, and transcript, then contribute one concise, focused turn (a few sentences). Long earlier turns are shown as a head/tail PREVIEW with a `file://` link — read that file for the full text only if the preview is not enough. Do not edit files or run mutating commands — return your meeting contribution as text only."

// Context-offloading budgets (LangChain Deep Agents pattern): recent turns are
// shown as full head/tail previews; older turns collapse to a one-line reference,
// so the prompt stays bounded no matter how many rounds run (which also avoids the
// argv-size fragility some agent CLIs hit on a long raw transcript).
const (
	previewFull = 520 // inline in full at or below this many chars
	previewHead = 380 // else: first N …
	previewTail = 120 // … + last N + a file:// link
	recentTurns = 8   // this many most-recent turns get a full preview
)

// preview renders one event for the replayed context: full text if short, else a
// head/tail excerpt with a file:// link to the complete turn (read-on-demand).
func preview(e Event) string {
	t := sanitizeTurn(e.Text)
	if len(t) <= previewFull || e.File == "" {
		return oneLine(t)
	}
	head := strings.TrimSpace(t[:previewHead])
	tail := strings.TrimSpace(t[len(t)-previewTail:])
	elided := len(t) - previewHead - previewTail
	return fmt.Sprintf("%s …[+%d chars — full: file://%s]… %s",
		oneLine(head), elided, e.File, oneLine(tail))
}

// briefRef is the collapsed form for older turns: a short lead + a file link.
func briefRef(e Event) string {
	t := oneLine(sanitizeTurn(e.Text))
	if len(t) > 120 {
		t = t[:120] + "…"
	}
	if e.File != "" {
		return fmt.Sprintf("%s [full: file://%s]", t, e.File)
	}
	return t
}

// ansiEscape matches ANSI CSI/OSC escape sequences that leak into an agent CLI's
// combined output (e.g. opencode's "\x1b[0m > build" banner).
var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)")

// sanitizeTurn makes captured agent output safe to STORE and to REPLAY as prompt
// context. Agent CLIs emit banners/warnings on the combined stream, sometimes with
// invalid UTF-8 (truncated box-drawing) and ANSI/control bytes. Fed back verbatim
// as the next agent's argv these crash downstream tools — codex exec rejects
// "invalid UTF-8 in arguments"; aider throws UnicodeEncodeError writing its input
// history. So: coerce to valid UTF-8, strip ANSI escapes and C0/C1 control chars
// (keeping \n and \t), and trim. Preserves ordinary prose (incl. legitimate
// non-ASCII).
func sanitizeTurn(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = ansiEscape.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r < 0x20 || (r >= 0x7f && r < 0xa0): // C0/C1 control
			return -1
		case r >= 0xd800 && r <= 0xdfff: // surrogates (never valid in UTF-8 argv)
			return -1
		case r >= 0x2500 && r <= 0x259f: // box-drawing + block elements — CLI banner chrome
			return -1
		case r == 0xfffd: // replacement char from an earlier lossy decode
			return -1
		default:
			return r
		}
	}, s)
	// Collapse runs of blank lines/spaces left by stripped banners.
	s = regexp.MustCompile(`[ \t]{2,}`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func transcriptContext(events []Event) string {
	// Keep only the relevant events (agenda is already in turnPrompt).
	rel := make([]Event, 0, len(events))
	for _, e := range events {
		if e.Kind != "agenda" {
			rel = append(rel, e)
		}
	}
	if len(rel) == 0 {
		return "(no turns yet)"
	}
	var b strings.Builder
	for i, e := range rel {
		who := e.Speaker
		switch e.Kind {
		case "decision":
			who = "DECISION"
		case "action":
			who = "ACTION"
		}
		// Decisions/actions (short + load-bearing) and the most recent turns get a
		// full preview; older turns collapse to a one-line reference.
		recent := i >= len(rel)-recentTurns
		if e.Kind == "decision" || e.Kind == "action" || recent {
			fmt.Fprintf(&b, "%s: %s\n", who, preview(e))
		} else {
			fmt.Fprintf(&b, "%s: %s\n", who, briefRef(e))
		}
	}
	return strings.TrimSpace(b.String())
}

func turnPrompt(st *State, question string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\nTopic: %s\n", turnGuard, st.Topic)
	if len(st.Agenda) > 0 {
		fmt.Fprintf(&b, "Agenda: %s\n", strings.Join(st.Agenda, " | "))
	}
	if q := strings.TrimSpace(question); q != "" {
		fmt.Fprintf(&b, "Current question: %s\n", q)
	}
	return strings.TrimSpace(b.String())
}

// turnTimeout parses the session's per-turn timeout (default 20m) so a wedged
// agent can't hang the round — no external `timeout` wrapper needed.
func turnTimeout(st *State) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(st.TurnTimeout)); err == nil && d > 0 {
		return d
	}
	return 20 * time.Minute
}

// runTurn invokes one participant and appends its turn to the transcript.
func runTurn(ctx context.Context, st *State, name, question string, runner chat.Runner) (Event, error) {
	events, _ := readTranscript(st.ID)
	res, err := chat.Invoke(ctx, chat.Options{
		Agent:       name,
		Instruction: turnPrompt(st, question),
		Context:     []string{transcriptContext(events)},
		Cwd:         st.Cwd,
		Timeout:     turnTimeout(st),
	}, runner)
	ev := Event{Round: st.Round, Speaker: name, Role: "participant", Kind: "turn", TS: nowFn()}
	if err != nil {
		// A failed turn is recorded as a SHORT marker — never the raw error
		// output (agent CLI banners/tracebacks would pollute the transcript and,
		// replayed as context, crash the next agent). It is not offloaded.
		ev.Text = fmt.Sprintf("(%s unavailable this turn: %s)", name, oneLine(sanitizeTurn(shortErr(res.Output, err))))
	} else {
		ev.Text = sanitizeTurn(res.Output)
		if ev.Text == "" {
			ev.Text = fmt.Sprintf("(%s returned no content)", name)
		} else {
			ev.File = writeTurnFile(st.ID, ev) // offload full text for read-on-demand
		}
	}
	if aerr := appendEvent(st.ID, ev); aerr != nil {
		return ev, aerr
	}
	return ev, err
}

// shortErr produces a compact failure reason from an agent's output+error,
// bounded so a multi-KB CLI banner never enters the transcript.
func shortErr(out string, err error) string {
	s := strings.TrimSpace(sanitizeTurn(out))
	if s == "" {
		s = err.Error()
	}
	s = oneLine(s)
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// runRound runs one sequential round across all participants.
func runRound(ctx context.Context, st *State, question string, runner chat.Runner) []Event {
	st.Round++
	_ = st.save()
	out := make([]Event, 0, len(st.Participants))
	for _, name := range st.Participants {
		ev, _ := runTurn(ctx, st, name, question, runner)
		out = append(out, ev)
	}
	return out
}

// record appends a marker/human event. Human/marker text is sanitized too — it
// is replayed as prompt context to agents, so stray control/invalid-UTF-8 bytes
// there would crash the same downstream tools as raw agent banners.
func record(st *State, kind, speaker, role, text string) (Event, error) {
	ev := Event{Round: st.Round, Speaker: speaker, Role: role, Kind: kind, Text: sanitizeTurn(text), TS: nowFn()}
	return ev, appendEvent(st.ID, ev)
}

func currentAgenda(st *State) string {
	if st.Round > 0 && st.Round <= len(st.Agenda) {
		return st.Agenda[st.Round-1]
	}
	if len(st.Agenda) > 0 {
		return st.Agenda[0]
	}
	return ""
}

func findRepoRoot(dir string) string {
	d := dir
	for d != "" {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	return ""
}

// minutesPath resolves where the published minutes go. An explicit --out path
// wins; otherwise docs/meetings/ inside a git repo, else the session store.
func minutesPath(st *State) string {
	out := strings.TrimSpace(st.Out)
	if out != "" && out != "docs" && out != "kb" {
		return out
	}
	ts := st.Created.Format("2006-01-02T15-04")
	name := fmt.Sprintf("meeting-note-%s-%s.md", ts, slugify(st.Topic))
	if root := findRepoRoot(st.Cwd); root != "" {
		return filepath.Join(root, "docs", "meetings", name)
	}
	dir, _ := storeDir(st.ID)
	return filepath.Join(dir, "minutes.md")
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

// renderMinutes builds the deterministic minutes document. Decisions and
// action-items come only from explicit markers (the secretary never decides);
// summary is an optional secretary-authored prose block.
func renderMinutes(st *State, events []Event, summary string, openQ []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Meeting — %s\n", st.Topic)
	fmt.Fprintf(&b, "Date: %s  ·  Session: %s\n", st.Created.Format("2006-01-02 15:04"), st.ID)
	attendees := []string{st.Human + " (human)"}
	for _, p := range st.Participants {
		attendees = append(attendees, p+" (participant)")
	}
	attendees = append(attendees, st.Secretary+" (secretary)")
	fmt.Fprintf(&b, "Attendees: %s\n\n", strings.Join(attendees, " · "))
	if len(st.Agenda) > 0 {
		fmt.Fprintf(&b, "Agenda: %s\n\n", strings.Join(st.Agenda, " · "))
	}

	var decisions, actions []string
	for _, e := range events {
		switch e.Kind {
		case "decision":
			decisions = append(decisions, e.Text)
		case "action":
			actions = append(actions, e.Text)
		}
	}
	b.WriteString("## Decisions\n")
	if len(decisions) == 0 {
		b.WriteString("(none recorded — discussed without an explicit /decision)\n")
	} else {
		for i, d := range decisions {
			fmt.Fprintf(&b, "%d. %s\n", i+1, d)
		}
	}
	b.WriteString("\n## Action items\n")
	if len(actions) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, a := range actions {
			fmt.Fprintf(&b, "- [ ] %s\n", a)
		}
	}
	if len(openQ) > 0 {
		b.WriteString("\n## Open questions\n")
		for _, q := range openQ {
			fmt.Fprintf(&b, "- %s\n", q)
		}
	}
	if s := strings.TrimSpace(summary); s != "" {
		fmt.Fprintf(&b, "\n## Summary\n%s\n", s)
	}
	b.WriteString("\n## Notes (turns)\n")
	for _, e := range events {
		if e.Kind == "turn" || e.Kind == "human" {
			fmt.Fprintf(&b, "- **%s:** %s\n", e.Speaker, oneLine(e.Text))
		}
	}
	dir, _ := storeDir(st.ID)
	fmt.Fprintf(&b, "\nTranscript: %s\n", filepath.Join(dir, "transcript.jsonl"))
	return b.String()
}

// hasMarkers reports whether the transcript already carries decision/action
// markers (so converge doesn't duplicate what a human already marked).
func hasMarkers(events []Event) bool {
	for _, e := range events {
		if e.Kind == "decision" || e.Kind == "action" {
			return true
		}
	}
	return false
}

// converge runs the secretary's synthesis pass: it EXTRACTS decisions, action
// items, and open questions as stated (never invents), records the decisions and
// actions as durable markers, and returns the open questions + a short summary.
// It is the difference between a filed transcript and filed minutes.
func converge(ctx context.Context, st *State, runner chat.Runner) (openQ []string, summary string) {
	if st.Secretary == "" {
		return nil, ""
	}
	events, _ := readTranscript(st.ID)
	instr := "You are the meeting secretary (notes only). From the transcript, extract ONLY what participants actually said — do not invent, do not decide, do not opine. Output EXACTLY these four sections, each as '- ' bullet lines (write 'none' if empty):\n" +
		"DECISIONS:\nACTIONS:\nOPEN QUESTIONS:\nSUMMARY:\n" +
		"(SUMMARY is 2-4 neutral sentences. An ACTION should name an owner if one was stated.)"
	res, err := chat.Invoke(ctx, chat.Options{
		Agent: st.Secretary, Role: "secretary", Instruction: instr,
		Context: []string{transcriptContext(events)}, Cwd: st.Cwd, Timeout: turnTimeout(st),
	}, runner)
	if err != nil {
		return nil, ""
	}
	clean := sanitizeTurn(res.Output)
	dec, act, oq, sum := parseConverge(clean)
	if sum == "" && len(dec) == 0 && len(act) == 0 && len(oq) == 0 {
		sum = strings.TrimSpace(clean) // unsectioned reply → treat as the summary
	}
	if !hasMarkers(events) { // don't double-record if a human already marked
		for _, d := range dec {
			_, _ = record(st, "decision", st.Secretary, "secretary", d)
		}
		for _, a := range act {
			_, _ = record(st, "action", st.Secretary, "secretary", a)
		}
	}
	return oq, sum
}

// parseConverge splits the secretary's labeled output into sections.
func parseConverge(s string) (dec, act, oq []string, summary string) {
	cur := ""
	var sumParts []string
	for line := range strings.SplitSeq(s, "\n") {
		l := strings.TrimSpace(line)
		switch up := strings.ToUpper(l); {
		case strings.HasPrefix(up, "DECISION"):
			cur = "d"
			continue
		case strings.HasPrefix(up, "ACTION"):
			cur = "a"
			continue
		case strings.HasPrefix(up, "OPEN QUESTION"):
			cur = "q"
			continue
		case strings.HasPrefix(up, "SUMMARY"):
			cur = "s"
			continue
		}
		if l == "" {
			continue
		}
		item := strings.TrimSpace(strings.TrimLeft(l, "-*•0123456789. "))
		if item == "" || strings.EqualFold(item, "none") {
			continue
		}
		switch cur {
		case "d":
			dec = append(dec, item)
		case "a":
			act = append(act, item)
		case "q":
			oq = append(oq, item)
		case "s":
			sumParts = append(sumParts, item)
		}
	}
	return dec, act, oq, strings.TrimSpace(strings.Join(sumParts, " "))
}

// closeMeeting converges (optionally) and writes the minutes.
func closeMeeting(ctx context.Context, st *State, synth bool, runner chat.Runner) (string, error) {
	var openQ []string
	summary := ""
	if synth {
		openQ, summary = converge(ctx, st, runner)
	}
	events, err := readTranscript(st.ID) // re-read: converge may have recorded markers
	if err != nil {
		return "", err
	}
	md := renderMinutes(st, events, summary, openQ)
	path := minutesPath(st)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := atomicWrite(path, []byte(md)); err != nil {
		return "", err
	}
	st.Status = "closed"
	_ = st.save()
	return path, nil
}
