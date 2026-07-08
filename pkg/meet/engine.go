package meet

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qiangli/coreutils/pkg/chat"
)

const turnGuard = "You are a participant in a planning meeting. Read the topic, agenda, and transcript, then contribute one concise, focused turn (a few sentences). Do not edit files or run commands — return meeting contribution text only."

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
	if len(events) == 0 {
		return "(no turns yet)"
	}
	var b strings.Builder
	for _, e := range events {
		if e.Kind == "agenda" {
			continue
		}
		who := e.Speaker
		if e.Kind == "decision" {
			who = "DECISION"
		} else if e.Kind == "action" {
			who = "ACTION"
		}
		fmt.Fprintf(&b, "%s: %s\n", who, oneLine(sanitizeTurn(e.Text)))
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

// runTurn invokes one participant and appends its turn to the transcript.
func runTurn(ctx context.Context, st *State, name, question string, runner chat.Runner) (Event, error) {
	events, _ := readTranscript(st.ID)
	res, err := chat.Invoke(ctx, chat.Options{
		Agent:       name,
		Instruction: turnPrompt(st, question),
		Context:     []string{transcriptContext(events)},
		Cwd:         st.Cwd,
	}, runner)
	text := sanitizeTurn(res.Output)
	if text == "" && err != nil {
		text = fmt.Sprintf("(no response: %v)", err)
	}
	ev := Event{Round: st.Round, Speaker: name, Role: "participant", Kind: "turn", Text: text, TS: nowFn()}
	if aerr := appendEvent(st.ID, ev); aerr != nil {
		return ev, aerr
	}
	return ev, err
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
func renderMinutes(st *State, events []Event, summary string) string {
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

// closeMeeting synthesizes (optionally) and writes the minutes.
func closeMeeting(ctx context.Context, st *State, synth bool, runner chat.Runner) (string, error) {
	events, err := readTranscript(st.ID)
	if err != nil {
		return "", err
	}
	summary := ""
	if synth && st.Secretary != "" {
		res, e := chat.Invoke(ctx, chat.Options{
			Agent:       st.Secretary,
			Role:        "secretary",
			Instruction: "You are the meeting secretary (notes only). Write a neutral 2-4 sentence summary of what was discussed and decided. Add no opinions and no new decisions.",
			Context:     []string{transcriptContext(events)},
			Cwd:         st.Cwd,
		}, runner)
		if e == nil {
			summary = strings.TrimSpace(res.Output)
		}
	}
	md := renderMinutes(st, events, summary)
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
