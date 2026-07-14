package meet

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/spf13/cobra"
)

// referenceMD is the full `bashy meet` guide, embedded into the binary so it is
// available wherever bashy runs (agents use bashy as a tool, not from this repo).
// Surfaced by `bashy meet reference`.
//
//go:embed reference.md
var referenceMD string

// meetDepthEnv bounds recursion. `meet` spawns agent CLIs, and those agents can
// call `bashy meet` themselves — so a panelist could convene a panel, whose
// panelists convene panels, forking exponentially and unboundedly. The depth
// marker is exported into every spawned agent's environment; convening from
// inside a meeting is refused.
const meetDepthEnv = "BASHY_MEET_DEPTH"

func meetDepth() int {
	n, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(meetDepthEnv)))
	return n
}

// guardDepth refuses to convene a meeting from inside one.
func guardDepth() error {
	if d := meetDepth(); d >= 1 {
		return fmt.Errorf("meet: refusing to convene a meeting from inside a meeting (%s=%d).\n"+
			"      A participant must contribute a turn, not convene its own panel — that recursion is unbounded.\n"+
			"      If you are an agent that needs a second opinion, say so in your turn and let the chair decide", meetDepthEnv, d)
	}
	return nil
}

// markDepth stamps the environment inherited by every agent this process spawns.
func markDepth() { _ = os.Setenv(meetDepthEnv, strconv.Itoa(meetDepth()+1)) }

// NewMeetCmd returns the `bashy meet` command tree.
func NewMeetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meet",
		Short: "multi-participant deliberation session with a notes-only secretary",
		Long: "Run a turn-taking planning meeting across agentic CLIs and a human.\n" +
			"A dedicated notes-only secretary keeps the minutes and files them to\n" +
			"docs/meetings/ on close. Agents can convene a one-shot panel with\n" +
			"`bashy meet consult`. Run `bashy meet reference` for the full guide.",
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.AddCommand(
		newStartCmd(), newConsultCmd(), newTellCmd(), newRoundCmd(),
		newPollCmd(), newAskCmd(),
		newConvergeCmd(), newCloseCmd(), newAmendCmd(), newApplyCmd(),
		newShowCmd(), newContributionsCmd(), newListCmd(), newResumeCmd(), newReferenceCmd(),
		newObserveCmd(), newSayCmd(),
	)
	return cmd
}

func newReferenceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reference",
		Short: "print the full bashy meet guide (embedded in the binary)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), referenceMD)
			return nil
		},
	}
}

func humanName() string {
	if u := strings.TrimSpace(os.Getenv("USER")); u != "" {
		return u
	}
	return "you"
}

func drivability(name string) string {
	// Shared operability gate with the capability router (pkg/capability): the
	// codex login-shell caveat is surfaced by shellRouting below, not here.
	// The name is resolved to its harness first — a participant seated by
	// nickname is still driven by a binary, and LookPath does not know the
	// nickname.
	if ok, _ := capability.Operable(capability.ResolveTool(name)); !ok {
		return "not installed"
	}
	return "installed"
}

// shellRouting reports whether a participant's shell commands will run through
// bashy. The chat launcher force-injects SHELL/CLAUDE_CODE_SHELL + a PATH shim
// into every spawned agent (on unless BASHY_FORCE_AGENT_SHELL=0), which covers
// every agent EXCEPT codex — it reads the /etc/passwd login shell, not the
// environment, so it needs `bashy install-agent codex` (chsh) instead.
func shellRouting(name string) string {
	if os.Getenv("BASHY_FORCE_AGENT_SHELL") == "0" {
		return "shell: system (forcing disabled)"
	}
	// Match on the harness, not on what the seat was called. `codex`,
	// `codex-gpt-5.5`, and the nickname drawn for that binding are all the
	// same binary reading the same /etc/passwd — a caveat that only fires for
	// one spelling of the name is a caveat that does not fire.
	if capability.ResolveTool(name) == "codex" {
		return "shell: ⚠ codex reads /etc/passwd — run `bashy install-agent codex` (chsh) to route via bashy"
	}
	return "shell: bashy ✓ (env-forced)"
}

func printPreview(w io.Writer, st *State) {
	fmt.Fprintln(w, "meet: resolved session")
	fmt.Fprintf(w, "  id           %s\n", st.ID)
	if st.Room > 0 {
		fmt.Fprintf(w, "  room         %d   ← `bashy meet observe %d` to watch it live\n", st.Room, st.Room)
	}
	fmt.Fprintf(w, "  initiator    %s\n", st.initiatorLabel())
	fmt.Fprintf(w, "  secretary    %s  records only, decides nothing — %s · %s\n", st.Secretary, drivability(st.Secretary), shellRouting(st.Secretary))
	if st.chaired() {
		fmt.Fprintf(w, "  chair        %s  directs, never argues — %s · %s\n", st.chair(), drivability(st.chair()), shellRouting(st.chair()))
	} else {
		fmt.Fprintln(w, "  chair        (none — round-robin; the human directs)")
	}
	for i, p := range st.Participants {
		label := "participants"
		if i > 0 {
			label = "            "
		}
		fmt.Fprintf(w, "  %s %s  %s · %s\n", label, seatLabel(p), drivability(p), shellRouting(p))
	}
	if len(st.Participants) == 0 {
		fmt.Fprintln(w, "  participants (none)")
	}
	fmt.Fprintf(w, "  human        %s\n", st.Human)
	if len(st.Context) > 0 {
		fmt.Fprintf(w, "  context      %s\n", strings.Join(st.Context, ", "))
	}
	dir, _ := storeDir(st.ID)
	fmt.Fprintf(w, "  store        %s/\n", redactHome(dir))
	fmt.Fprintf(w, "  minutes →    %s\n", redactHome(minutesPath(st)))
	fmt.Fprintf(w, "  turn model   %s · decisions=%s\n", st.turnModel(), st.decisionMode())
	for _, warn := range attendeeWarnings(st) {
		fmt.Fprintf(w, "  ⚠ %s\n", warn)
	}
}

// attendeeWarnings applies the meet attendee gate (see
// dhnt/docs/agentic-capability-taxonomy.md §meet attendee requirements): flag
// non-routable participants (operability) and a roster past the Self-MoA sweet
// spot (diversity). Advisory — it warns, it does not refuse.
func attendeeWarnings(st *State) []string {
	var out []string
	for _, p := range st.Participants {
		if ok, reason := capability.Operable(capability.ResolveTool(p)); !ok {
			out = append(out, fmt.Sprintf("%s is not routable: %s", p, reason))
		}
	}
	if n := len(st.Participants); n > 4 {
		out = append(out, fmt.Sprintf("%d participants exceeds the 2–4 Self-MoA sweet spot — trim redundant seats (same tool/model dilutes signal)", n))
	}
	return out
}

// sessionFlags are shared by `start` and `consult`.
type sessionFlags struct {
	topic        string
	secretary    string
	chair        string
	out          string
	turnTimeout  string
	decisionMode string
	initiator    string
	minTurnChars int
	maxTurns     int
	maxStalls    int
	minBand      int
	steerable    bool
	participants []string
	agenda       []string
	context      []string
	rosterNotes  []string // what --min-band seated, and what it could not
}

func (sf *sessionFlags) bind(cmd *cobra.Command) {
	f := cmd.Flags()
	f.StringVar(&sf.topic, "topic", "", "meeting topic (required)")
	f.StringArrayVar(&sf.participants, "participant", nil, "participant agent — decides content (repeatable)")
	f.IntVar(&sf.minBand, "min-band", 0, "seat every operable agent at this capability band or above (1-4), instead of naming them")
	f.StringVar(&sf.secretary, "secretary", "claude", "secretary agent — records, decides nothing; never a participant or the chair")
	f.StringVar(&sf.chair, "chair", "", "chair agent — directs the discussion and judges done-ness; empty means round-robin with no chair")
	f.StringArrayVar(&sf.agenda, "agenda", nil, "agenda item (repeatable)")
	f.StringArrayVar(&sf.context, "context", nil, "file every participant reads before its first turn (repeatable)")
	f.IntVar(&sf.maxTurns, "max-turns", defaultMaxTurns, "hard ceiling on participant turns under a --chair")
	f.IntVar(&sf.maxStalls, "max-stalls", defaultMaxStalls, "consecutive looping/no-progress turns before the chair re-plans")
	f.StringVar(&sf.out, "out", "docs", "filing target: docs | kb | <path>")
	f.StringVar(&sf.turnTimeout, "turn-timeout", "20m", "per-turn agent timeout (e.g. 20m); a wedged agent can't hang the round")
	f.StringVar(&sf.decisionMode, "decision-mode", "infer", "infer: the secretary may record a converged decision (tagged); explicit: only stated decisions")
	f.StringVar(&sf.initiator, "initiator", "", "who convened the meeting and must confirm it may end; must be someone at the table (default: the human)")
	f.IntVar(&sf.minTurnChars, "min-turn-chars", 0, "a reply shorter than N chars counts as `short`, not a contribution")
	f.BoolVar(&sf.steerable, "steerable", false,
		"hold each speaker OPEN for its whole turn, so `meet say` reaches it mid-answer. "+
			"Without this a turn is a headless one-shot: it runs the prompt and exits, so a steer arrives "+
			"after the agent is already gone. Costs a TUI startup and a silence timeout per turn — a live "+
			"turn has no exit to end it, so it ends on quiet")
}

func (sf *sessionFlags) newState() (*State, error) {
	for _, f := range sf.context {
		if _, err := os.Stat(f); err != nil {
			return nil, fmt.Errorf("meet: --context %s: %w", f, err)
		}
	}
	// Seating happens before Validate, so a band-built roster is held to the
	// same rules as one someone typed out.
	if err := sf.seatByBand(); err != nil {
		return nil, err
	}
	// And every seat is resolved to its canonical name before Validate, so the
	// duplicate check sees through aliases: --participant Sable --participant
	// claude-fable5 is ONE agent seated twice, and seating it twice would
	// dilute the vote while looking like diversity.
	sf.canonicalizeRoster()
	cwd, _ := os.Getwd()
	st := &State{
		ID: newID(sf.topic, nowFn()), Room: assignRoom(), Topic: sf.topic, Agenda: sf.agenda,
		Participants: sf.participants, Secretary: sf.secretary, Chair: sf.chair,
		Human:        humanName(),
		Initiator:    sf.initiator,
		DecisionMode: sf.decisionMode, MinTurnChars: sf.minTurnChars, Context: sf.context,
		Steerable: sf.steerable,
		MaxTurns:  sf.maxTurns, MaxStalls: sf.maxStalls,
		Status: "open", Cwd: cwd, Out: sf.out,
		TurnTimeout: sf.turnTimeout, Created: nowFn(),
	}
	if err := st.Validate(); err != nil {
		return nil, err
	}
	return st, nil
}

// deliberate runs the discussion under whichever turn model the roster implies,
// so `start --non-interactive` and `consult` share one path. There is no mode
// flag: an agent chair runs the ledger loop, no chair runs a round-robin.
func deliberate(ctx context.Context, st *State, w io.Writer, rounds int, question string, verbose bool) error {
	if st.chaired() {
		res, err := runChaired(ctx, st, nil)
		if err != nil {
			return err
		}
		if verbose {
			fmt.Fprintf(w, "chaired: %d turns, %d stalls, %d re-plans, %d degraded selections — stopped by %s\n",
				res.Turns, res.Stalls, res.Replans, res.Degraded, res.StoppedBy)
		}
		if res.StoppedBy == "stalled" {
			fmt.Fprintln(w, "⚠ the meeting stalled — participants stopped adding anything new")
		}
		return nil
	}
	for i := 0; i < rounds; i++ {
		q := question
		if q == "" && i < len(st.Agenda) {
			q = st.Agenda[i]
		}
		for _, e := range runRound(ctx, st, q, nil) {
			if verbose {
				fmt.Fprintf(w, "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
		}
	}
	return nil
}

func newStartCmd() *cobra.Command {
	var sf sessionFlags
	var rounds int
	var dry, nonInteractive, yes bool
	cmd := &cobra.Command{
		Use:   "start --topic TEXT [--participant AGENT ...]",
		Short: "start a meeting (enters the REPL unless --non-interactive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guardDepth(); err != nil {
				return err
			}
			if strings.TrimSpace(sf.initiator) == "" {
				sf.initiator = humanName() // `start` always names its initiator
			}
			st, err := sf.newState()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			sf.printRoster(w)
			printPreview(w, st)
			if dry {
				fmt.Fprintln(w, "  (dry-run: no agents launched)")
				return nil
			}
			if err := st.save(); err != nil {
				return err
			}
			markDepth()
			for _, a := range st.Agenda {
				_, _ = record(st, "agenda", procedural(st), string(RoleChair), a)
			}
			if !nonInteractive {
				return repl(cmd, st)
			}
			if err := deliberate(cmd.Context(), st, w, rounds, "", true); err != nil {
				return err
			}
			// An unattended run cannot prompt a human; an agent initiator is still
			// asked, because that is the whole point of agent-initiated meetings.
			autoYes := yes || st.initiatorKind() == "human"
			path, err := closeMeeting(cmd.Context(), st, closeOptions{
				Synthesize: true, Confirm: true, Yes: autoYes,
				In: cmd.InOrStdin(), Out: w,
			}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "wrote %s\n", redactHome(path))
			return nil
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.IntVar(&rounds, "rounds", 1, "rounds to run in --non-interactive mode")
	f.BoolVar(&dry, "dry-run", false, "print the resolved session and exit")
	f.BoolVar(&nonInteractive, "non-interactive", false, "run rounds then close, no REPL")
	f.BoolVar(&yes, "yes", false, "close without asking the initiator to confirm")
	return cmd
}

// Consult outcomes. Disagreement is a FIRST-CLASS result, not an error: the one
// thing a calling agent must never do is read `summary` from a panel that did not
// converge and act on it as an answer.
const (
	verdictAgree    = "agree"    // decisive, complete panel — safe to act on
	verdictSplit    = "split"    // the panel genuinely disagreed — you decide
	verdictEscalate = "escalate" // the panel could not answer (seats failed, or nothing decided)
)

// Exit codes, mirroring the convention agent-callable gates converge on:
// 0 = act on it · 1 = a decision exists but blocking issues were raised ·
// 2 = do not proceed on this alone.
const (
	exitAgree    = 0
	exitBlocked  = 1
	exitEscalate = 2
)

// Verdict is the machine-readable result of a one-shot `meet consult`. It is the
// return value of `meet` used as a tool: a calling agent reads this, not prose.
type Verdict struct {
	Schema        string      `json:"schema"`
	ID            string      `json:"id"`
	Topic         string      `json:"topic"`
	Question      string      `json:"question,omitempty"`
	Participants  []string    `json:"participants"`
	Rounds        int         `json:"rounds"`
	Verdict       string      `json:"verdict"`    // agree | split | escalate
	Confidence    float64     `json:"confidence"` // 0..1, from panel coverage and vote share
	ExitCode      int         `json:"exit_code"`
	Summary       string      `json:"summary,omitempty"`
	Decisions     []Decision  `json:"decisions,omitempty"`
	Actions       []string    `json:"actions,omitempty"`
	Risks         []string    `json:"risks,omitempty"` // the blocking issues
	OpenQuestions []string    `json:"open_questions,omitempty"`
	Corrections   []string    `json:"corrections,omitempty"`
	Poll          *PollResult `json:"poll,omitempty"`
	Coverage      []Coverage  `json:"coverage"`
	Minutes       string      `json:"minutes"`
}

// decide computes the verdict, a confidence, and an exit code from what actually
// happened — never from a model's self-reported confidence, which the literature
// finds badly miscalibrated. Confidence here is a coverage-and-vote-share
// statistic: what fraction of the seats we asked actually answered, and how
// lopsidedly.
func (v *Verdict) decide() {
	seats := len(v.Coverage)
	answered := 0
	for _, c := range v.Coverage {
		if c.Contributed() {
			answered++
		}
	}
	coverageRatio := 1.0
	if seats > 0 {
		coverageRatio = float64(answered) / float64(seats)
	}

	// A poll, when present, is the sharpest signal we have.
	agreement := 0.5
	decisive := false
	if v.Poll != nil {
		if win, ok := v.Poll.Winner(); ok {
			decisive = true
			if seats > 0 {
				agreement = float64(v.Poll.Tally[win]) / float64(seats)
			}
		} else {
			agreement = 0.0
		}
	} else if len(v.Decisions) > 0 {
		decisive = true
		agreement = 1.0
	} else {
		agreement = 0.0
	}

	v.Confidence = coverageRatio * agreement

	switch {
	case answered < seats || answered == 0:
		// Half a panel is a sample, not a consensus, however loudly it agreed.
		v.Verdict, v.ExitCode = verdictEscalate, exitEscalate
	case len(v.Decisions) == 0:
		// The panel answered but settled nothing — there is no result to split over.
		v.Verdict, v.ExitCode = verdictEscalate, exitEscalate
	case !decisive:
		v.Verdict, v.ExitCode = verdictSplit, exitEscalate
	case len(v.Risks) > 0:
		v.Verdict, v.ExitCode = verdictAgree, exitBlocked
	default:
		v.Verdict, v.ExitCode = verdictAgree, exitAgree
	}
}

// newConsultCmd is `meet` as a tool call: one command, no REPL, no confirmation
// round-trip (the caller IS the initiator and receives the verdict synchronously),
// and a JSON verdict on stdout. An agent mid-task runs this to get a cross-vendor
// second opinion and then continues.
func newConsultCmd() *cobra.Command {
	var sf sessionFlags
	var question, deadline string
	var choices []string
	var rounds int
	var jsonOut, failOnDissent bool
	cmd := &cobra.Command{
		Use:   "consult --topic TEXT [--question TEXT] [--participant AGENT ...]",
		Short: "one-shot panel: convene, deliberate, synthesize, return a verdict (agent-callable)",
		Long: "Convene a panel, run the rounds, poll if --choice is given, synthesize, file the\n" +
			"minutes, and print a verdict. Blocks until done and never enters a REPL, so an\n" +
			"agentic tool can call it as a tool and read the result.\n\n" +
			"A participant cannot call this from inside a meeting (unbounded recursion).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := guardDepth(); err != nil {
				return err
			}
			// The caller is an agent we cannot name unless it says so. Leave the
			// initiator empty rather than inventing an attendee; consult never
			// confirms, because the caller receives the verdict synchronously.
			if q := strings.TrimSpace(question); q != "" && len(sf.agenda) == 0 {
				sf.agenda = []string{q}
			}
			st, err := sf.newState()
			if err != nil {
				return err
			}
			// consult's stdout is the verdict a caller parses; the roster note
			// is commentary and belongs on stderr.
			sf.printRoster(cmd.ErrOrStderr())
			if len(st.Participants) == 0 {
				return fmt.Errorf("meet: consult needs at least one --participant or a --min-band")
			}
			if err := st.save(); err != nil {
				return err
			}
			markDepth()
			w := cmd.OutOrStdout()

			// A blocking call needs a ceiling on the WHOLE consult, not just on
			// each turn: N participants × R rounds × --turn-timeout is otherwise a
			// multi-hour hang inside somebody's tool call.
			ctx := cmd.Context()
			if d, err := time.ParseDuration(strings.TrimSpace(deadline)); err == nil && d > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, d)
				defer cancel()
			} else if strings.TrimSpace(deadline) != "" {
				return fmt.Errorf("meet: bad --deadline %q: %w", deadline, err)
			}

			for _, a := range st.Agenda {
				_, _ = record(st, "agenda", procedural(st), string(RoleChair), a)
			}
			if err := deliberate(ctx, st, w, rounds, question, false); err != nil {
				return err
			}

			v := &Verdict{
				Schema: schemaVersion, ID: st.ID, Topic: st.Topic, Question: question,
				Participants: st.Participants,
			}
			if len(choices) > 0 {
				poll, err := runPoll(ctx, st, question, choices, st.Participants, nil)
				if err != nil {
					return err
				}
				for i := range poll.Votes {
					poll.Votes[i].Text = redactHome(poll.Votes[i].Text)
					poll.Votes[i].File = redactHome(poll.Votes[i].File)
				}
				v.Poll = poll
			}

			// The caller is the initiator and gets the verdict synchronously, so
			// there is nobody else to confirm the conclusion to.
			path, err := closeMeeting(ctx, st, closeOptions{
				Synthesize: true, Confirm: false, Out: io.Discard,
			}, nil)
			if err != nil {
				return err
			}
			events, _ := readTranscript(st.ID)
			if syn := loadSynthesis(st.ID); syn != nil {
				v.Summary, v.Decisions, v.Actions = syn.Summary, syn.Decisions, syn.Actions
				v.Risks, v.OpenQuestions, v.Corrections = syn.Risks, syn.OpenQuestions, syn.Corrections
			}
			v.Rounds, v.Coverage, v.Minutes = st.Round, coverage(st, events), redactHome(path)
			v.decide()

			if jsonOut {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				if err := enc.Encode(v); err != nil {
					return err
				}
			} else {
				writeVerdict(w, v)
			}
			if failOnDissent && v.ExitCode != exitAgree {
				return fmt.Errorf("meet: verdict=%s (exit_code=%d) — the panel did not agree", v.Verdict, v.ExitCode)
			}
			return nil
		},
	}
	sf.bind(cmd)
	f := cmd.Flags()
	f.StringVar(&question, "question", "", "the question put to the panel")
	f.StringArrayVar(&choices, "choice", nil, "make it a poll: permitted answer (repeatable; default free-form)")
	f.IntVar(&rounds, "rounds", 1, "deliberation rounds before the poll/synthesis")
	f.StringVar(&deadline, "deadline", "10m", "hard ceiling on the whole consult (a blocking tool call must not hang)")
	f.BoolVar(&jsonOut, "json", false, "emit the verdict as JSON (the agent-callable shape)")
	f.BoolVar(&failOnDissent, "fail-on-dissent", false, "exit non-zero unless the verdict is `agree` with no blocking issues")
	return cmd
}

func writeVerdict(w io.Writer, v *Verdict) {
	fmt.Fprintf(w, "meeting %s — %s\n\n", v.ID, v.Topic)
	if v.Summary != "" {
		fmt.Fprintf(w, "%s\n\n", v.Summary)
	}
	if v.Poll != nil {
		if win, ok := v.Poll.Winner(); ok {
			fmt.Fprintf(w, "poll: %s\n", win)
		} else {
			fmt.Fprintf(w, "poll: no clear result\n")
		}
		for _, vote := range v.Poll.Votes {
			answer := vote.Choice
			if answer == "" {
				answer = statusOf(vote)
			}
			fmt.Fprintf(w, "  %-12s %s\n", vote.Speaker, answer)
		}
		fmt.Fprintln(w)
	}
	for _, d := range v.Decisions {
		tag := ""
		if d.Inferred {
			tag = " (inferred)"
		}
		fmt.Fprintf(w, "decision: %s%s\n", d.Text, tag)
	}
	for _, a := range v.Actions {
		fmt.Fprintf(w, "action:   %s\n", a)
	}
	for _, r := range v.Risks {
		fmt.Fprintf(w, "risk:     %s\n", r)
	}
	for _, q := range v.OpenQuestions {
		fmt.Fprintf(w, "open:     %s\n", q)
	}
	fmt.Fprintf(w, "\nverdict: %s (confidence %.2f, exit %d)\n", v.Verdict, v.Confidence, v.ExitCode)
	switch v.Verdict {
	case verdictSplit:
		fmt.Fprintln(w, "⚠ the panel genuinely disagreed — this is input, not an answer")
	case verdictEscalate:
		fmt.Fprintln(w, "⚠ the panel could not answer (seats failed, or nothing was decided) — do not act on this alone")
	}
	fmt.Fprintf(w, "\nminutes: %s\n", v.Minutes)
}

// repl is the interactive meeting loop.
func repl(cmd *cobra.Command, st *State) error {
	w := cmd.OutOrStdout()
	sc := bufio.NewScanner(cmd.InOrStdin())
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	fmt.Fprintf(w, "\nmeet %s · secretary=%s(notes-only) · participants: %s\n",
		st.ID, st.Secretary, strings.Join(st.Participants, ", "))
	fmt.Fprintln(w, "commands: <text> | @name <text> | /round | /chair | /poll <q> | /ask <q> |")
	fmt.Fprintln(w, "          /decision <t> | /action owner: task | /agenda <t> | /show | /converge | /close")
	fmt.Fprint(w, "you> ")
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			fmt.Fprint(w, "you> ")
			continue
		}
		switch {
		case line == "/converge":
			syn, err := converge(cmd.Context(), st, nil)
			if err != nil {
				fmt.Fprintf(w, "⏺ secretary pass failed: %v\n", err)
				break
			}
			fmt.Fprintf(w, "⏺ converged: %d decisions, %d actions, %d risks, %d open questions\n",
				len(syn.Decisions), len(syn.Actions), len(syn.Risks), len(syn.OpenQuestions))
			if syn.Summary != "" {
				fmt.Fprintf(w, "  summary: %s\n", oneLine(syn.Summary))
			}
		case line == "/show":
			events, _ := readTranscript(st.ID)
			writeShow(w, st, events, loadSynthesis(st.ID))
		case line == "/close":
			path, err := closeMeeting(cmd.Context(), st, closeOptions{
				Synthesize: true, Confirm: true, In: cmd.InOrStdin(), Out: w,
			}, nil)
			if errors.Is(err, ErrDeclined) {
				fmt.Fprintln(w, "⏺ meeting continues.")
				break
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "wrote %s\n", redactHome(path))
			return nil
		case line == "/round":
			for _, e := range runRound(cmd.Context(), st, currentAgenda(st), nil) {
				fmt.Fprintf(w, "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
		case line == "/chair":
			if !st.chaired() {
				fmt.Fprintln(w, "⏺ no --chair agent for this meeting; use /round, or restart with --chair <agent>")
				break
			}
			res, err := runChaired(cmd.Context(), st, nil)
			if err != nil {
				fmt.Fprintf(w, "⏺ chairing failed: %v\n", err)
				break
			}
			fmt.Fprintf(w, "⏺ chaired %d turns (%d stalls, %d re-plans, %d degraded) — stopped by %s\n",
				res.Turns, res.Stalls, res.Replans, res.Degraded, res.StoppedBy)
		case strings.HasPrefix(line, "/poll "):
			q := strings.TrimSpace(line[len("/poll "):])
			res, err := runPoll(cmd.Context(), st, q, nil, nil, nil)
			if err != nil {
				fmt.Fprintf(w, "⏺ poll failed: %v\n", err)
				break
			}
			for _, v := range res.Votes {
				answer := v.Choice
				if answer == "" {
					answer = statusOf(v)
				}
				fmt.Fprintf(w, "%s> %s — %s\n", v.Speaker, answer, oneLine(v.Text))
			}
			if win, ok := res.Winner(); ok {
				fmt.Fprintf(w, "⏺ poll result: %s\n", win)
			} else {
				fmt.Fprintln(w, "⏺ poll result: no clear result")
			}
		case strings.HasPrefix(line, "/ask "):
			q := strings.TrimSpace(line[len("/ask "):])
			evs, err := runAsk(cmd.Context(), st, q, true, nil, nil)
			if err != nil {
				fmt.Fprintf(w, "⏺ ask failed: %v\n", err)
				break
			}
			for _, e := range evs {
				fmt.Fprintf(w, "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
		case strings.HasPrefix(line, "/decision "):
			t := strings.TrimSpace(line[len("/decision "):])
			_, _ = record(st, "decision", st.Human, "", t)
			fmt.Fprintf(w, "⏺ recorded DECISION: %s\n", t)
		case strings.HasPrefix(line, "/action "):
			t := strings.TrimSpace(line[len("/action "):])
			_, _ = record(st, "action", st.Human, "", t)
			fmt.Fprintf(w, "⏺ recorded ACTION: %s\n", t)
		case strings.HasPrefix(line, "/agenda "):
			t := strings.TrimSpace(line[len("/agenda "):])
			st.Agenda = append(st.Agenda, t)
			_ = st.save()
			_, _ = record(st, "agenda", procedural(st), string(RoleChair), t)
		case strings.HasPrefix(line, "@"):
			name, text, _ := strings.Cut(strings.TrimPrefix(line, "@"), " ")
			ev, _ := runTurn(cmd.Context(), st, name, text, nil)
			fmt.Fprintf(w, "%s> %s\n", ev.Speaker, oneLine(ev.Text))
		default:
			_, _ = record(st, "human", st.Human, "human", line)
		}
		fmt.Fprint(w, "you> ")
	}
	return sc.Err()
}

func newTellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tell <id> <text...>",
		Short: "append a human contribution to a session",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			_, err = record(st, "human", st.Human, "human", strings.Join(args[1:], " "))
			return err
		},
	}
}

func newRoundCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "round <id>",
		Short: "run one moderated round across participants",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			markDepth()
			for _, e := range runRound(cmd.Context(), st, currentAgenda(st), nil) {
				fmt.Fprintf(cmd.OutOrStdout(), "%s> %s\n", e.Speaker, oneLine(e.Text))
			}
			return nil
		},
	}
}

// newPollCmd is the request-for-comment style: a fixed answer set, every
// participant must answer, the tally is recorded.
func newPollCmd() *cobra.Command {
	var question string
	var choices, participants []string
	cmd := &cobra.Command{
		Use:   "poll <id> --question TEXT [--choice yes --choice no]",
		Short: "put a fixed-choice question to the participants and tally the answers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			markDepth()
			res, err := runPoll(cmd.Context(), st, question, choices, participants, nil)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, v := range res.Votes {
				answer := v.Choice
				if answer == "" {
					answer = statusOf(v)
				}
				fmt.Fprintf(w, "%-12s %-10s %s\n", v.Speaker, answer, oneLine(v.Text))
			}
			if win, ok := res.Winner(); ok {
				fmt.Fprintf(w, "\nresult: %s\n", win)
			} else {
				fmt.Fprintln(w, "\nresult: no clear result (tie, or too many non-answers)")
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&question, "question", "", "the poll question (required)")
	f.StringArrayVar(&choices, "choice", nil, "permitted answer (repeatable; default: yes, no)")
	f.StringArrayVar(&participants, "participant", nil, "poll only these participants (default: all)")
	return cmd
}

// newAskCmd is the open-question style: answering is optional and silence is a
// recorded abstention rather than a failure.
func newAskCmd() *cobra.Command {
	var question string
	var participants []string
	var required bool
	cmd := &cobra.Command{
		Use:   "ask <id> --question TEXT",
		Short: "put an open question to the participants (answering is optional)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			markDepth()
			evs, err := runAsk(cmd.Context(), st, question, !required, participants, nil)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, e := range evs {
				fmt.Fprintf(w, "── %s (%s)\n%s\n\n", e.Speaker, statusOf(e), strings.TrimSpace(e.Text))
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&question, "question", "", "the open question (required)")
	f.StringArrayVar(&participants, "participant", nil, "ask only these participants (default: all)")
	f.BoolVar(&required, "required", false, "an empty answer is a failure, not an abstention")
	return cmd
}

func newConvergeCmd() *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "converge <id>",
		Short: "secretary pass: extract decisions, actions, risks, open questions, corrections",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			if mode != "" {
				st.DecisionMode = mode
				_ = st.save()
			}
			markDepth()
			syn, err := converge(cmd.Context(), st, nil)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "converged (%s mode): %d decisions, %d actions, %d risks, %d open questions, %d corrections\n",
				syn.Mode, len(syn.Decisions), len(syn.Actions), len(syn.Risks), len(syn.OpenQuestions), len(syn.Corrections))
			if syn.Summary != "" {
				fmt.Fprintf(w, "summary: %s\n", oneLine(syn.Summary))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mode, "decision-mode", "", "override the session's decision mode: infer | explicit")
	return cmd
}

func newCloseCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "close <id>",
		Short: "secretary pass, confirm with the initiator, then write and file the minutes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			markDepth()
			path, err := closeMeeting(cmd.Context(), st, closeOptions{
				Synthesize: true, Confirm: true, Yes: yes,
				In: cmd.InOrStdin(), Out: cmd.OutOrStdout(),
			}, nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", redactHome(path))
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "close without asking the initiator to confirm")
	return cmd
}

// newAmendCmd re-runs the secretary over the existing transcript and rewrites the
// minutes. The fix for a weak secretary pass: the transcript is the durable
// artifact, the minutes are a projection of it, and a projection can be redone.
func newAmendCmd() *cobra.Command {
	var mode string
	var resynthesize bool
	cmd := &cobra.Command{
		Use:   "amend <id>",
		Short: "regenerate the minutes from the transcript (optionally re-running the secretary)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			if mode != "" {
				st.DecisionMode = mode
				resynthesize = true
			}
			w := cmd.OutOrStdout()
			if resynthesize {
				markDepth()
				if _, err := converge(cmd.Context(), st, nil); err != nil {
					return err
				}
				fmt.Fprintf(w, "re-ran the secretary (%s mode)\n", st.decisionMode())
			}
			_ = st.save()
			path, err := fileMinutes(st)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "rewrote %s\n", redactHome(path))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&mode, "decision-mode", "", "re-run the secretary with this mode: infer | explicit")
	f.BoolVar(&resynthesize, "resynthesize", false, "re-run the secretary before rewriting the minutes")
	return cmd
}

func newApplyCmd() *cobra.Command {
	var to string
	var write bool
	cmd := &cobra.Command{
		Use:   "apply <id> [--to PATH --write]",
		Short: "render the agreed action items as a block; --write appends them to a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			events, _ := readTranscript(st.ID)
			block, err := applyActions(st, events, loadSynthesis(st.ID), to, write)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if write {
				fmt.Fprintf(w, "appended %d action item(s) to %s\n", len(actionsOf(events, loadSynthesis(st.ID))), to)
				return nil
			}
			fmt.Fprint(w, block)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&to, "to", "", "target document")
	f.BoolVar(&write, "write", false, "append the block to --to (default: print it)")
	return cmd
}

func newShowCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "show <room>|<id>",
		Short: "show a meeting's roster, per-participant coverage, and artifacts",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveMeeting(args[0])
			if err != nil {
				return err
			}
			args = []string{id}
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			events, _ := readTranscript(st.ID)
			w := cmd.OutOrStdout()
			if jsonOut {
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"state": st, "coverage": coverage(st, events), "synthesis": loadSynthesis(st.ID),
				})
			}
			writeShow(w, st, events, loadSynthesis(st.ID))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the session, coverage, and synthesis as JSON")
	return cmd
}

func newContributionsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:     "contributions <id> [participant]",
		Aliases: []string{"contrib"},
		Short:   "print every contribution in full, optionally filtered to one participant",
		Args:    cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := loadState(args[0])
			if err != nil {
				return err
			}
			who := ""
			if len(args) == 2 {
				who = args[1]
			}
			events, _ := readTranscript(st.ID)
			w := cmd.OutOrStdout()
			if jsonOut {
				var sel []Event
				for _, e := range events {
					if e.Kind != "turn" && e.Kind != "vote" && e.Kind != "human" {
						continue
					}
					if who != "" && !strings.EqualFold(e.Speaker, who) {
						continue
					}
					e.Text = redactHome(e.Text)
					e.File = redactHome(e.File)
					sel = append(sel, e)
				}
				enc := json.NewEncoder(w)
				enc.SetIndent("", "  ")
				return enc.Encode(sel)
			}
			writeContributions(w, st, events, who)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit the contributions as JSON")
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list saved meetings",
		Long: "List saved meetings.\n\n" +
			"ROOM is the short number you attach by: `bashy meet observe 2`. It is\n" +
			"assigned from the lowest free number among the OPEN meetings and reused\n" +
			"once a meeting closes, exactly like a shell's job numbers.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := openRooms()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ROOM\tID\tSTATUS\tPARTICIPANTS\tTOPIC")
			for _, s := range sessions {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					roomLabel(s), s.ID, s.Status, strings.Join(s.Participants, ","), s.Topic)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if len(sessions) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "no meetings on this host")
			}
			return nil
		},
	}
}

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume <room>|<id>",
		Short: "reopen a saved meeting in the REPL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := resolveMeeting(args[0])
			if err != nil {
				return err
			}
			st, err := loadState(id)
			if err != nil {
				return err
			}
			markDepth()
			return repl(cmd, st)
		},
	}
}
