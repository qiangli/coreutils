package meet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/qiangli/coreutils/pkg/agentctl"
	"github.com/qiangli/coreutils/pkg/capability"
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

// ansiEscape matches the escape sequences that leak into an agent CLI's captured
// output.
//
// Three alternatives, and the first two both matter:
//
//   - CSI — `\x1b[ … final`. The parameter class INCLUDES the private markers
//     `<>=?`, which the original pattern omitted. Under a pty a tool resets the
//     terminal on exit and emits `\x1b[>4m` and `\x1b[<u`; without the private
//     markers those did not match, and the tail of every claude turn was recorded
//     as literal `(B[>4m[<u78`.
//   - OSC — `\x1b] … BEL|ST`.
//   - Two-character escapes — `\x1b7`, `\x1b8` (save/restore cursor) and
//     `\x1b(B` (charset select). Not CSI at all, so nothing else catches them.
//
// This runs on the recorded turn AND on each live line, so a watcher and the
// transcript never disagree about what was said.
var ansiEscape = regexp.MustCompile(
	"\x1b\\[[0-9;?<>=]*[ -/]*[@-~]" + // CSI, incl. private-mode params
		"|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)" + // OSC
		"|\x1b[()][0-9A-Za-z]" + // charset select: ESC ( B
		"|\x1b[0-9A-Za-z><=]") // two-char: ESC 7, ESC 8, ESC =

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
		case "ledger":
			who = "CHAIR"
		case "replan":
			who = "CHAIR (new approach)"
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

// isTimeout distinguishes a per-turn deadline from an ordinary crash. The agent
// is killed by exec's context, which surfaces as "signal: killed" rather than
// context.DeadlineExceeded, so the elapsed time is the reliable signal — with a
// slack margin because the kill races the deadline.
func isTimeout(err error, elapsed, budget time.Duration) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return budget > 0 && elapsed >= budget-2*time.Second
}

// invokeAgent runs one agent turn and classifies the outcome. The classification
// is the whole point: "opencode returned no content" is useless to an operator
// who cannot tell a timeout from a crash from a considered silence.
func invokeAgent(ctx context.Context, st *State, name, role, instruction, question string, runner chat.Runner) (Event, error) {
	events, _ := readTranscript(st.ID)
	budget := turnTimeout(st)
	start := time.Now()

	// Tee this turn onto the live channel so an observer watches the argument
	// being made rather than being told, minutes later, that it was made. It is
	// a tee: `res` is unchanged, so the RECORDED turn is byte-for-byte what it
	// would have been with nobody watching.
	//
	// A terminal is given only to a tool that can USE one — the registry says
	// which (agentctl.Profile.NeedsTerminal): claude listens mid-run and has a
	// trust prompt to clear, so it gets a pty and a control socket; codex and agy
	// declare supports_say=false, so they get a pipe.
	//
	// This is not tidiness. A pty merges stdout and stderr, so a tool's chrome —
	// codex prints a version banner and its workdir — lands inside the captured
	// answer, where a pipe keeps it out. That cost is worth paying for an agent a
	// chair can interrupt, and pure loss for one that would only sit there being
	// un-steerable and noisy.
	//
	// The other half of the contract is that print mode is DECLARED, never
	// inferred. An agent CLI that decides it is headless by sniffing whether
	// stdout is a terminal is right on a pipe and wrong on a pty — claude opened
	// its REPL and sat there. A terminal changes what an agent can be ASKED; it
	// must never change what the agent DOES.
	var sock string
	usePTY := false
	if p, ok := agentctl.ProfileFor(toolOf(name)); ok && p.NeedsTerminal() {
		usePTY = true
		sock = ctlSockPath(st, name)
	}
	live := newLiveWriter(st, name, role, sock)
	res, err := chat.Invoke(ctx, chat.Options{
		Agent:       name,
		Role:        role,
		Instruction: instruction,
		Context:     []string{transcriptContext(events)},
		Files:       st.Context, // every participant reviews the same source set
		Cwd:         st.Cwd,
		Timeout:     budget,
		Stream:      live,
		PTY:         usePTY,
		CtlSock:     sock,

		// A MEETING IS A CONVERSATION, NOT A WORK SESSION. Every seat here —
		// participant, chair, secretary — produces exactly one thing: text, which
		// this process captures and writes. No attendee has any reason to touch
		// the filesystem, and the context files it reviews are read INTO the
		// prompt for it (Files, above), so it does not even need to open them.
		//
		// The same integrity argument as `bashy judge`, one step earlier: an
		// agent that can edit the code it is discussing can quietly "fix" the
		// thing under debate and then argue from the fixed version, and the
		// minutes would record a discussion of a codebase that no longer exists.
		//
		// This is also what makes a meeting work OUT OF THE BOX. The launch guard
		// refuses to strip an agent CLI's approval gate on an uncontained host —
		// correctly, since that hands an unattended agent full access. Read-only
		// removes the dangerous flags rather than demanding permission to keep
		// them, so the guard passes BY CONSTRUCTION: there is nothing left to
		// guard. Nobody has to set BASHY_ALLOW_UNSAFE_AGENT_LAUNCH to hold a
		// meeting, and nobody has to weaken a host to watch one.
		ReadOnly: true,
	}, runner)
	elapsed := time.Since(start)

	ev := Event{
		Round: st.Round, Speaker: name, Role: string(RoleParticipant), Kind: "turn",
		Question: question, TS: nowFn(),
		ExitCode: res.ExitCode, DurMS: elapsed.Milliseconds(),
	}
	switch {
	case err != nil && isTimeout(err, elapsed, budget):
		ev.Status = statusTimeout
		ev.Text = fmt.Sprintf("(%s timed out after %s)", name, budget)
	case err != nil:
		// A failed turn is recorded as a SHORT marker — never the raw error
		// output (agent CLI banners/tracebacks would pollute the transcript and,
		// replayed as context, crash the next agent). It is not offloaded.
		ev.Status = statusError
		ev.Text = fmt.Sprintf("(%s unavailable this turn: %s)", name, oneLine(sanitizeTurn(shortErr(res.Output, err))))
	default:
		text := sanitizeTurn(res.Output)
		ev.Chars = len(text)
		switch {
		case text == "":
			ev.Status = statusEmpty
			ev.Text = fmt.Sprintf("(%s returned no content)", name)
		case st.MinTurnChars > 0 && len(text) < st.MinTurnChars:
			ev.Status = statusShort
			ev.Text = text
			ev.File = writeTurnFile(st.ID, ev)
		default:
			ev.Status = statusOK
			ev.Text = text
			ev.File = writeTurnFile(st.ID, ev) // offload full text for read-on-demand
		}
	}
	// Free the floor, and say how the turn ended. A watcher must be able to tell
	// a timeout from a crash from a considered silence — the same distinction the
	// transcript preserves, which is why the status is carried on both channels.
	live.close(ev.Status)
	return ev, err
}

// runTurn invokes one participant and appends its turn to the transcript.
func runTurn(ctx context.Context, st *State, name, question string, runner chat.Runner) (Event, error) {
	ev, err := invokeAgent(ctx, st, name, "", turnPrompt(st, question), question, runner)
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

// minutesTurnChars bounds one turn's full text in the published minutes. The
// per-turn file always holds the complete bytes, so nothing is lost — but the
// minutes must carry the ARGUMENT, not a 240-char ellipsis of it.
const minutesTurnChars = 4000

// blockquote renders a turn's full text as a markdown blockquote, capped, with a
// pointer to the complete per-turn file when it was truncated.
func blockquote(text, file string) string {
	t := strings.TrimSpace(text)
	truncated := false
	if len(t) > minutesTurnChars {
		t = strings.TrimSpace(t[:minutesTurnChars])
		truncated = true
	}
	var b strings.Builder
	for line := range strings.SplitSeq(t, "\n") {
		fmt.Fprintf(&b, "> %s\n", line)
	}
	if truncated {
		if file != "" {
			fmt.Fprintf(&b, ">\n> *(truncated — full text: `%s`)*\n", redactHome(file))
		} else {
			b.WriteString(">\n> *(truncated)*\n")
		}
	}
	return b.String()
}

// renderMinutes builds the deterministic minutes document.
//
// Decisions and action items come from explicit human markers in the transcript
// PLUS the secretary's synthesis; inferred decisions are labelled so a reader can
// always tell what was stated from what was read out of a consensus.
func renderMinutes(st *State, events []Event, syn *Synthesis) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Meeting — %s\n", st.Topic)
	fmt.Fprintf(&b, "Date: %s  ·  Session: `%s`\n", st.Created.Format("2006-01-02 15:04"), st.ID)
	fmt.Fprintf(&b, "Initiator: %s\n", st.initiatorLabel())

	attendees := []string{st.Human + " (human)"}
	for _, p := range st.Participants {
		attendees = append(attendees, p+" (participant)")
	}
	if st.chaired() {
		attendees = append(attendees, st.chair()+" (chair)")
	}
	attendees = append(attendees, st.Secretary+" (secretary)")
	fmt.Fprintf(&b, "Attendees: %s\n", strings.Join(attendees, " · "))
	if len(st.Context) > 0 {
		fmt.Fprintf(&b, "Context reviewed: %s\n", strings.Join(st.Context, " · "))
	}
	b.WriteString("\n")
	if len(st.Agenda) > 0 {
		fmt.Fprintf(&b, "Agenda: %s\n\n", strings.Join(st.Agenda, " · "))
	}

	if syn != nil && strings.TrimSpace(syn.Summary) != "" {
		fmt.Fprintf(&b, "## Summary\n%s\n\n", redactHome(syn.Summary))
	}

	// Explicit human markers first — they are authoritative and never inferred.
	var decisions []Decision
	var actions []string
	for _, e := range events {
		switch e.Kind {
		case "decision":
			decisions = append(decisions, Decision{Text: e.Text})
		case "action":
			actions = append(actions, e.Text)
		}
	}
	if syn != nil {
		decisions = append(decisions, syn.Decisions...)
		actions = append(actions, syn.Actions...)
	}

	b.WriteString("## Decisions\n")
	if len(decisions) == 0 {
		b.WriteString("(none — the meeting reached no decision)\n")
	} else {
		for i, d := range decisions {
			tag := ""
			if d.Inferred {
				tag = " *(inferred from consensus"
				if len(d.Support) > 0 {
					tag += "; agreed: " + strings.Join(d.Support, ", ")
				}
				tag += ")*"
			}
			fmt.Fprintf(&b, "%d. %s%s\n", i+1, redactHome(d.Text), tag)
		}
	}

	b.WriteString("\n## Action items\n")
	if len(actions) == 0 {
		b.WriteString("(none)\n")
	} else {
		for _, a := range actions {
			fmt.Fprintf(&b, "- [ ] %s\n", redactHome(a))
		}
	}

	if syn != nil {
		writeList(&b, "Risks", syn.Risks)
		writeList(&b, "Open questions", syn.OpenQuestions)
		writeList(&b, "Corrections / revised framing", syn.Corrections)
	}

	renderPolls(&b, events)

	b.WriteString("\n## Participant coverage\n\n")
	writeCoverageTable(&b, coverage(st, events))

	b.WriteString("\n## Notes (turns)\n")
	for _, e := range events {
		switch e.Kind {
		case "human":
			fmt.Fprintf(&b, "\n**%s** (human):\n\n%s", e.Speaker, blockquote(redactHome(e.Text), ""))
		case "turn":
			status := statusOf(e)
			label := fmt.Sprintf("\n**%s** (round %d", e.Speaker, e.Round)
			if status != statusOK {
				label += ", " + status
			}
			label += "):\n\n"
			b.WriteString(label)
			b.WriteString(blockquote(redactHome(e.Text), e.File))
		case "confirm":
			fmt.Fprintf(&b, "\n**%s** (conclusion confirmed): %s\n", e.Speaker, oneLine(redactHome(e.Text)))
		case "ledger":
			fmt.Fprintf(&b, "\n*chair: %s*\n", oneLine(redactHome(e.Text)))
		case "replan":
			fmt.Fprintf(&b, "\n**%s** (chair re-plan after a stall):\n\n%s", e.Speaker, blockquote(redactHome(e.Text), e.File))
		case "note":
			fmt.Fprintf(&b, "\n*%s*\n", oneLine(redactHome(e.Text)))
		}
	}

	dir, _ := storeDir(st.ID)
	fmt.Fprintf(&b, "\nTranscript: `%s`\n", redactHome(filepath.Join(dir, "transcript.jsonl")))
	return b.String()
}

func writeList(b *strings.Builder, title string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n## %s\n", title)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", redactHome(it))
	}
}

// turnFailed reports whether a turn event failed to contribute. An abstention on
// an optional question is NOT a failure.
func turnFailed(e Event) bool { return !contributed(e) }

// recordOperability folds each participant's per-turn outcome into the capability
// matrix's operability column (the self-updating loop). Operability is
// tool-governed, so a clean turn is a pass and a failure marker is a fail for that
// tool — a flaky agent nets down over the meeting.
func recordOperability(st *State, events []Event) {
	seat := map[string]bool{}
	for _, p := range st.Participants {
		seat[p] = true
	}
	for _, e := range events {
		if e.Kind != "turn" && e.Kind != "vote" {
			continue
		}
		if !seat[e.Speaker] {
			continue
		}
		// An abstention on an optional question is a clean turn: the tool ran and
		// the agent chose to say nothing. Only real failures net down.
		_ = capability.RecordOperability(e.Speaker, !turnFailed(e))
	}
}

// convergeInstruction builds the secretary's prompt. The decision mode is the
// load-bearing knob: `explicit` extracts only decisions a participant stated
// outright, `infer` additionally lets the secretary name a decision the meeting
// clearly converged on — which is what stops a meeting that plainly agreed on
// four things from filing "Decisions: none recorded".
//
// The guard against hallucinated consensus is not the mode, it is the label: an
// inferred decision is marked as such, and inference is still forbidden from
// inventing a position no participant took.
func convergeInstruction(mode string) string {
	var b strings.Builder
	b.WriteString("You are the meeting secretary (notes only). Read the transcript and report what happened. " +
		"You do not participate, propose, vote, or decide.\n\n")
	if mode == "explicit" {
		b.WriteString("DECISIONS: list ONLY decisions a participant stated outright as decided. " +
			"If the meeting converged on something but nobody declared it a decision, it is an OPEN QUESTION, not a decision.\n\n")
	} else {
		b.WriteString("DECISIONS: list decisions a participant stated outright, AND decisions the meeting clearly " +
			"converged on. A convergence needs BOTH a proposal and an acceptance: one participant proposed it and at " +
			"least one other AGREED, and nobody dissented. Prefix every converged-but-undeclared item with the literal " +
			"token (inferred) and end it with the literal token [agreed: name1, name2] naming the participants who " +
			"proposed and agreed. Discussing an option is NOT agreeing to it. Never invent a position no participant " +
			"actually took — if you are unsure whether it was agreed, it is an OPEN QUESTION, not a decision.\n\n")
	}
	b.WriteString("Output EXACTLY these sections, each as '- ' bullet lines, writing 'none' when empty:\n" +
		"DECISIONS:\nACTIONS:\nRISKS:\nOPEN QUESTIONS:\nCORRECTIONS:\nSUMMARY:\n\n" +
		"ACTIONS name an owner when one was stated. RISKS are hazards a participant raised. " +
		"CORRECTIONS are claims in the topic, agenda, or earlier framing that the meeting corrected or superseded — " +
		"so a reader does not mistake stale framing for an endorsed premise. " +
		"SUMMARY is 2-4 neutral sentences.")
	return b.String()
}

// converge runs the secretary's synthesis pass and persists the result to
// synthesis.json (latest pass wins). Safe to re-run: it never appends markers to
// the transcript, so `meet amend` cannot duplicate anything.
func converge(ctx context.Context, st *State, runner chat.Runner) (*Synthesis, error) {
	if st.Secretary == "" {
		return nil, fmt.Errorf("meet: no secretary configured for %s", st.ID)
	}
	events, _ := readTranscript(st.ID)
	res, err := chat.Invoke(ctx, chat.Options{
		Agent: st.Secretary, Role: string(RoleSecretary), Instruction: convergeInstruction(st.decisionMode()),
		Context: []string{transcriptContext(events)}, Cwd: st.Cwd, Timeout: turnTimeout(st),
	}, runner)
	if err != nil {
		return nil, fmt.Errorf("meet: secretary %s failed: %w", st.Secretary, err)
	}
	syn := parseConverge(sanitizeTurn(res.Output))
	syn.demoteUnsupported() // an inferred decision with no named acceptance is an open question
	syn.By, syn.At, syn.Mode = st.Secretary, nowFn(), st.decisionMode()
	if err := syn.save(st.ID); err != nil {
		return nil, err
	}
	return syn, nil
}

// supportTag matches the trailing `[agreed: codex, claude]` grounding token.
var supportTag = regexp.MustCompile(`(?i)\[\s*agreed\s*:\s*([^\]]*)\]`)

// parseDecision pulls the `(inferred)` prefix and the `[agreed: …]` support list
// off one decision line.
func parseDecision(item string) Decision {
	d := Decision{Text: item}
	if m := supportTag.FindStringSubmatch(d.Text); m != nil {
		for _, name := range strings.Split(m[1], ",") {
			if n := strings.TrimSpace(name); n != "" {
				d.Support = append(d.Support, n)
			}
		}
		d.Text = strings.TrimSpace(supportTag.ReplaceAllString(d.Text, ""))
	}
	if rest, ok := cutPrefixFold(d.Text, "(inferred)"); ok {
		d.Inferred = true
		d.Text = strings.TrimSpace(rest)
	}
	d.Text = strings.TrimSpace(strings.Trim(d.Text, "—- "))
	return d
}

// cutPrefixFold is strings.CutPrefix, case-insensitive.
func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// parseConverge splits the secretary's labeled output into a Synthesis. An
// unsectioned reply degrades to a summary rather than being dropped.
func parseConverge(s string) *Synthesis {
	syn := &Synthesis{}
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
		case strings.HasPrefix(up, "RISK"):
			cur = "r"
			continue
		case strings.HasPrefix(up, "OPEN QUESTION"):
			cur = "q"
			continue
		case strings.HasPrefix(up, "CORRECTION"):
			cur = "c"
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
			syn.Decisions = append(syn.Decisions, parseDecision(item))
		case "a":
			syn.Actions = append(syn.Actions, item)
		case "r":
			syn.Risks = append(syn.Risks, item)
		case "q":
			syn.OpenQuestions = append(syn.OpenQuestions, item)
		case "c":
			syn.Corrections = append(syn.Corrections, item)
		case "s":
			sumParts = append(sumParts, item)
		}
	}
	syn.Summary = strings.TrimSpace(strings.Join(sumParts, " "))
	if syn.Summary == "" && len(syn.Decisions)+len(syn.Actions)+len(syn.Risks)+len(syn.OpenQuestions)+len(syn.Corrections) == 0 {
		syn.Summary = strings.TrimSpace(s) // unsectioned reply → treat as the summary
	}
	return syn
}

// ErrDeclined is returned when the initiator refuses to conclude the meeting.
var ErrDeclined = errors.New("meet: initiator declined to conclude the meeting")

// isTerminal reports whether r is an interactive terminal we may prompt on.
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// confirmConclusion asks the meeting's INITIATOR whether it may end, and records
// the answer. A human initiator is prompted on the terminal; an agent initiator
// is asked through the secretary's own channel, which is what lets an agent
// convene a meeting as a tool call and stay in control of when it ends.
//
// `yes` is the operator's explicit unattended override; it is recorded, not
// silent.
func confirmConclusion(ctx context.Context, st *State, in io.Reader, out io.Writer, yes bool, runner chat.Runner) error {
	who, kind := st.initiatorName(), st.initiatorKind()
	if yes {
		if who == "" {
			who = "unnamed caller"
		}
		_, _ = record(st, "confirm", who, kind, "concluded without prompting (--yes)")
		return nil
	}
	// An unnamed initiator is the agent that called `meet consult`, which receives
	// the verdict synchronously and never confirms. Reaching here means someone
	// asked an anonymous caller to confirm; there is nobody to ask.
	if who == "" {
		return fmt.Errorf("meet: this meeting has no named initiator to confirm it may end; " +
			"pass --initiator <name at the table>, or --yes for an unattended close")
	}

	events, _ := readTranscript(st.ID)
	syn := loadSynthesis(st.ID)
	var dec, act int
	if syn != nil {
		dec, act = len(syn.Decisions), len(syn.Actions)
	}
	brief := fmt.Sprintf("%d rounds, %d turns, %d decisions, %d action items", st.Round, countKind(events, "turn"), dec, act)

	if kind == "agent" {
		instr := fmt.Sprintf(
			"You convened this meeting (topic: %q). It has run %s. The secretary's synthesis is in the transcript below.\n\n"+
				"Has the meeting achieved what you convened it for? Reply on the FIRST line with EXACTLY one word: "+
				"CONCLUDE or CONTINUE. On the next line give one sentence of reason.", st.Topic, brief)
		ev, err := invokeAgent(ctx, st, who, "", instr, "conclude?", runner)
		if err != nil {
			return fmt.Errorf("meet: could not reach initiator %s to confirm conclusion: %w", who, err)
		}
		verdict, reason := parseVerdict(ev.Text)
		_, _ = record(st, "confirm", who, "agent", fmt.Sprintf("%s — %s", verdict, reason))
		if verdict != "CONCLUDE" {
			fmt.Fprintf(out, "meet: initiator %s asked to CONTINUE: %s\n", who, reason)
			return ErrDeclined
		}
		return nil
	}

	if !isTerminal(in) {
		return fmt.Errorf("meet: %s must confirm the meeting may end, but stdin is not a terminal; pass --yes for an unattended close", who)
	}
	fmt.Fprintf(out, "\nmeet: %s — %s\nEnd the meeting and file the minutes? [y/N] ", st.ID, brief)
	sc := bufio.NewScanner(in)
	if !sc.Scan() {
		return fmt.Errorf("meet: no confirmation received; pass --yes for an unattended close")
	}
	switch strings.ToLower(strings.TrimSpace(sc.Text())) {
	case "y", "yes":
		_, _ = record(st, "confirm", who, "human", "confirmed at the prompt")
		return nil
	}
	return ErrDeclined
}

// parseVerdict pulls CONCLUDE/CONTINUE plus a reason out of an agent's reply.
// Defaults to CONTINUE: an unparseable answer must not end someone's meeting.
func parseVerdict(text string) (verdict, reason string) {
	verdict = "CONTINUE"
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, l := range lines {
		up := strings.ToUpper(l)
		switch {
		case strings.Contains(up, "CONCLUDE"):
			verdict = "CONCLUDE"
		case strings.Contains(up, "CONTINUE"):
			verdict = "CONTINUE"
		default:
			continue
		}
		reason = strings.TrimSpace(strings.Join(lines[i+1:], " "))
		if reason == "" {
			reason = oneLine(text)
		}
		return verdict, oneLine(reason)
	}
	return verdict, "no CONCLUDE/CONTINUE verdict in the reply — defaulting to CONTINUE"
}

func countKind(events []Event, kind string) int {
	n := 0
	for _, e := range events {
		if e.Kind == kind {
			n++
		}
	}
	return n
}

// closeOptions controls the close path. Synthesize runs the secretary; Confirm
// gates the close on the initiator's agreement.
type closeOptions struct {
	Synthesize bool
	Confirm    bool
	Yes        bool
	In         io.Reader
	Out        io.Writer
}

// closeMeeting converges, asks the initiator to confirm, then writes the minutes.
func closeMeeting(ctx context.Context, st *State, opt closeOptions, runner chat.Runner) (string, error) {
	if opt.Out == nil {
		opt.Out = io.Discard
	}
	if opt.Synthesize {
		if _, err := converge(ctx, st, runner); err != nil {
			fmt.Fprintf(opt.Out, "meet: ⚠ secretary pass failed, filing without a synthesis: %v\n", err)
		}
	}
	if opt.Confirm {
		if err := confirmConclusion(ctx, st, opt.In, opt.Out, opt.Yes, runner); err != nil {
			return "", err
		}
	}
	return fileMinutes(st)
}

// fileMinutes renders and writes the minutes from whatever is on disk. It is the
// shared tail of `close` and `amend`.
func fileMinutes(st *State) (string, error) {
	events, err := readTranscript(st.ID)
	if err != nil {
		return "", err
	}
	recordOperability(st, events) // self-update the capability matrix from this meeting
	md := renderMinutes(st, events, loadSynthesis(st.ID))
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
