package weave

// THE TIME-BOX — the half of a sprint the kanban column cannot express.
//
// A column says WHERE work is (backlog, doing, review, done). Nothing says how
// long it has. That gap is not cosmetic when the worker is an agent: a session
// has no natural end. It does not get hungry, notice the light change, or feel
// a day turning into an evening. Left alone it will follow the next reasonable
// thread, and the one after that, and each step is defensible while the whole
// drifts far from what was agreed.
//
// So a sprint gets a START and a CUTOFF, and the cutoff is a DECISION POINT
// rather than a kill switch. Refusing to work past it would be wrong — the
// gate might be one fix from green, and stopping there wastes the run. What
// the cutoff buys is that the moment ARRIVES VISIBLY instead of passing
// unnoticed: every sprint command says how long is left, and says OVERDUE
// after, so continuing is a choice somebody makes rather than a default nobody
// observed.
//
// # Cadence comes from the record, not the plan
//
// `stop` writes what actually happened next to what was planned. That is the
// only thing that makes a cadence real: a two-hour box that consistently runs
// four hours is not a two-hour cadence, it is a four-hour cadence with a
// misleading label, and only the record can tell you which one you have.
//
// # Many boxes at once, each on its own clock
//
// The box lives on the CARD, never in a global "current sprint". There is no
// single active sprint anywhere in this package, and that is deliberate: real
// work runs several initiatives side by side on different rhythms — a 45-minute
// fix box beside a 4-hour migration box beside something opened yesterday and
// still going. Each start/stop cycle is independent, and starting one says
// nothing about any other.
//
// The one thing `start` refuses is restarting THE SAME card while its box is
// still running, because that would discard the original estimate — the single
// number the record exists to keep honest.
//
// # Why this is separate from the conductor lease
//
// The lease already carries a timestamp, and reusing it would have been
// tempting. It answers a different question. The lease asks "is a conductor
// still alive?" — a liveness heartbeat that a graceful handoff clears and a
// successor takes over. The box asks "should this work still be running?" —
// which stays true across a handoff, because the commitment belongs to the
// sprint and not to whoever is currently holding it. Collapsing them would
// mean a conductor switch silently reset the deadline.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// DefaultSprintBox is the cadence when none is given. Two hours is chosen to
// be a REVIEW interval rather than a day's work: long enough to finish
// something and short enough that a wrong direction is caught while it is
// still cheap to abandon.
const DefaultSprintBox = 2 * time.Hour

// weaveStoryBox is a sprint's time commitment.
//
// StoppedAt being nil is what makes a sprint "running", so the zero value is
// correctly "never started" rather than "started at the epoch".
type weaveStoryBox struct {
	StartedAt time.Time  `json:"started_at"`
	Cutoff    time.Time  `json:"cutoff"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	// Planned is stored rather than derived from Cutoff-StartedAt so that
	// extending a running box keeps the ORIGINAL commitment visible. What was
	// promised and what was taken are different facts, and a cadence tuned from
	// the second one alone would only ever ratchet upward.
	Planned time.Duration `json:"planned"`
	// Draining records when a graceful stop began. It survives a FAILED drain,
	// which is the point: workers are already parked, and the sprint stays open
	// in this state until the gate goes green. A conductor returning to it can
	// see the stop was already attempted rather than starting the reasoning over.
	Draining *time.Time `json:"draining,omitempty"`
	// GateRan / GatePassed are the evidence the stop was clean. Both false means
	// UNVERIFIED, never "fine" — absence of evidence is not success.
	GateRan    bool   `json:"gate_ran,omitempty"`
	GatePassed bool   `json:"gate_passed,omitempty"`
	GateCmd    string `json:"gate_cmd,omitempty"`
}

// Running reports a box that has started and not stopped.
func (b *weaveStoryBox) Running() bool {
	return b != nil && !b.StartedAt.IsZero() && b.StoppedAt == nil
}

// Remaining is time left before the cutoff; negative once past it.
func (b *weaveStoryBox) Remaining(now time.Time) time.Duration {
	if b == nil {
		return 0
	}
	return b.Cutoff.Sub(now)
}

// Overdue reports a running box past its cutoff.
func (b *weaveStoryBox) Overdue(now time.Time) bool {
	return b.Running() && now.After(b.Cutoff)
}

// Elapsed is how long the box has actually been open — to the stop if it
// stopped, to now if it is still running.
func (b *weaveStoryBox) Elapsed(now time.Time) time.Duration {
	if b == nil || b.StartedAt.IsZero() {
		return 0
	}
	if b.StoppedAt != nil {
		return b.StoppedAt.Sub(b.StartedAt)
	}
	return now.Sub(b.StartedAt)
}

// Status is the one-line marker every sprint surface shows, so the cutoff is
// never something a caller has to remember to ask about.
//
// An empty string means there is nothing to say — a sprint that was never
// boxed reads exactly as it did before this existed.
func (b *weaveStoryBox) Status(now time.Time) string {
	switch {
	case b == nil || b.StartedAt.IsZero():
		return ""
	case b.StoppedAt != nil:
		return fmt.Sprintf("stopped after %s (planned %s)",
			roundDur(b.Elapsed(now)), roundDur(b.Planned))
	case b.Overdue(now):
		// Stated as time PAST the cutoff, not as a negative remaining: "overdue
		// by 40m" is a fact somebody can act on, "-40m left" is arithmetic they
		// have to finish themselves.
		return fmt.Sprintf("OVERDUE by %s (box was %s)",
			roundDur(now.Sub(b.Cutoff)), roundDur(b.Planned))
	default:
		return fmt.Sprintf("%s left of %s", roundDur(b.Remaining(now)), roundDur(b.Planned))
	}
}

// roundDur trims a duration to something a human reads at a glance. Sub-minute
// precision on a two-hour box is noise, but it matters on a box measured in
// minutes, so the unit follows the magnitude.
func roundDur(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	d = d.Round(time.Minute)
	h, m := int(d/time.Hour), int((d%time.Hour)/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		// "4h", not Go's "4h0m0s" — a cadence is read at a glance dozens of
		// times a day, and the zero components are pure noise in every one.
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

// newSprintStartCmd opens a sprint's time-box.
func newSprintStartCmd() *cobra.Command {
	var flags weaveOutputFlags
	var forDur time.Duration
	var as string
	cmd := &cobra.Command{
		Use:   "start <sprint>",
		Short: "Open a sprint's time-box: start the clock and set a cutoff",
		Long: "start commits a sprint to a length of time.\n\n" +
			"The cutoff is a DECISION POINT, not a kill switch. Nothing refuses to run\n" +
			"past it — the gate might be one fix from green, and stopping there wastes\n" +
			"the run. What it buys is that the moment ARRIVES VISIBLY: every sprint\n" +
			"command reports the time left, and says OVERDUE after, so continuing is a\n" +
			"choice somebody makes rather than a default nobody noticed.\n\n" +
			"This matters most when the worker is an agent. A session has no natural\n" +
			"end — it will follow the next reasonable thread, and the one after that,\n" +
			"each step defensible while the whole drifts from what was agreed.\n\n" +
			"A sprint in `backlog` also moves to `doing`, because starting the clock and\n" +
			"starting the work are the same act.",
		Example: "  bashy sprint start 3            # the default cadence\n" +
			"  bashy sprint start 3 --for 45m\n" +
			"  bashy sprint start 3 --for 4h",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if forDur <= 0 {
				return fmt.Errorf("--for must be positive (got %s)", forDur)
			}
			now := time.Now().UTC()
			return runWeaveStoryMutate(cmd, id, "sprint start", &flags, func(s *weaveStory) (string, error) {
				// Restarting a RUNNING box would silently discard the original
				// commitment, which is the one number the record exists to keep
				// honest. Extending is a different, explicit act.
				if s.currentBox().Running() {
					return "", fmt.Errorf("sprint #%d is already running (%s) — `sprint stop %d` first, or `sprint extend %d --by <dur>`",
						id, s.currentBox().Status(now), id, id)
				}
				// A BOX IN FLIGHT MUST HAVE AN OWNER. The conductor holding the
				// lease is the sprint's scrum master: accountable for its
				// delivery, for calling the stop, and for the decision when the
				// cutoff arrives. A running box nobody holds is how a deadline
				// passes with everyone assuming someone else was watching — and
				// with several sprints running at once, that is the normal way
				// to lose one rather than an unlucky one.
				//
				// So start CLAIMS a free (or stale) lease, and refuses to take a
				// live one from someone else: quietly reassigning delivery
				// ownership is not something a start command should do.
				who := weaveStoryConductorName(s, as)
				if prev, stale, free := weaveStoryLeaseState(s); !free && !stale && prev != who {
					return "", fmt.Errorf("sprint #%d is held by %s — `sprint take %d` to assume delivery first",
						id, prev, id)
				}
				// AN OWNER MUST BE AN ADDRESS. Refuse a name that resolves to
				// no agent: every coordination surface (mb, chat, inbox, ping)
				// keys on this string, and one that names nobody is worse than
				// none — it is printed as a contact and silently never answers.
				// CLAIM-TIME: the seat must be RUNNING, not merely declared.
				// A sprint seated to a name with no process behind it accepts
				// room messages and inbox mail that nobody will ever read.
				if err := validateSprintClaimant(who); err != nil {
					return "", err
				}
				s.Lease = &weaveStoryLease{Holder: who, At: now}
				s.Owner = who
				s.Boxes = append(s.Boxes, weaveStoryBox{StartedAt: now, Cutoff: now.Add(forDur), Planned: forDur})
				// A room is opened automatically, because an OPTIONAL room is
				// empty exactly when it is needed: at the moment somebody
				// urgent arrives and the conductor is mid-turn elsewhere.
				// Failure is not fatal — a conductor with no intercom still
				// has a box to run, and the surfaces say "no contact" rather
				// than implying one.
				roomNote := ""
				if s.Contact == nil {
					c, cerr := openSprintRoom(s, who)
					switch {
					case cerr != nil:
						// Reported, never swallowed. A contact that silently
						// failed to open reads identically to one nobody has
						// tried to use yet, and the difference matters at
						// exactly the moment someone needs to reach in.
						roomNote = fmt.Sprintf("; no room (%v)", cerr)
					default:
						s.Contact = c
						roomNote = "; " + c.String()
					}
				} else {
					roomNote = "; " + s.Contact.String()
				}
				moved := ""
				if s.Column == "backlog" {
					s.Column = "doing"
					moved = " (backlog → doing)"
				}
				weaveStoryAppend(s, who, "system",
					fmt.Sprintf("started a %s box, cutoff %s", roundDur(forDur), now.Add(forDur).Format(time.RFC3339)))
				return fmt.Sprintf("sprint #%d started%s — %s, cutoff %s; conducted by %s%s",
					id, moved, roundDur(forDur), now.Add(forDur).Format("15:04 MST"), who, roomNote), nil
			})
		},
	}
	cmd.Flags().DurationVar(&forDur, "for", DefaultSprintBox, "how long this sprint gets")
	cmd.Flags().StringVar(&as, "as", "", "conductor name (default $BASHY_PRINCIPAL/$WEAVE_CONDUCTOR/$WEAVE_AGENT, then current lease holder)")
	flags.attach(cmd)
	return cmd
}

// newSprintStopCmd closes the box and records what actually happened.
func newSprintStopCmd() *cobra.Command {
	return newSprintCloseCmd(false)
}

// newSprintEndCmd closes the lifecycle, not merely the current time-box.
// It deliberately has no --force or --no-verify escape hatch: "done" must
// mean that linked work is parked, repositories are wrapped, and a real gate
// passed. Callers that only need to close a cadence cycle retain `stop`.
func newSprintEndCmd() *cobra.Command {
	return newSprintCloseCmd(true)
}

func newSprintCloseCmd(ending bool) *cobra.Command {
	var flags weaveOutputFlags
	var note, gateCmd, gateDir string
	var gateTimeout time.Duration
	var force, noVerify bool
	verb := "stop"
	short := "Close a sprint's time-box and record planned vs actual"
	if ending {
		verb = "end"
		short = "Finish a sprint after workers, repositories, and gates are consistent"
	}
	op := "sprint " + verb
	long := "stop DRAINS a sprint: it parks the workers, proves the tree still builds,\n"
	if ending {
		long = "end DRAINS and closes the sprint lifecycle: it parks linked workers, requires wrapped repositories and a green gate, moves the card to done, and releases the conductor lease.\n\n"
	}
	cmd := &cobra.Command{
		Use:   verb + " <sprint>",
		Short: short,
		Long: long +
			"and only then closes the clock.\n\n" +
			"The reason is preemption. Something more urgent arrives, this work must\n" +
			"yield, and it must yield in a state somebody can pick up cold. A stop is not\n" +
			"graceful because it was polite — it is graceful because three things are TRUE\n" +
			"when it finishes: no worker is still running (workspace and branch preserved,\n" +
			"exactly as `weave pause` leaves them), the tree compiles and its tests pass,\n" +
			"and there is a continuity record saying where it left off.\n\n" +
			"A RED GATE REFUSES TO CLOSE THE SPRINT, and that is the feature. The workers\n" +
			"are already parked so nothing is burning, and the remaining job is narrow:\n" +
			"fix the regression and run stop again. Closing over a red gate would file the\n" +
			"sprint as done and hand the next one damage it cannot tell from its own.\n\n" +
			"SPRINT WORK GOES THROUGH WEAVE, NEVER DIRECT EDITS. That is what makes a\n" +
			"hard stop survivable: unmerged branches cannot break the tree, so parking\n" +
			"them costs nothing — and a red gate therefore means something ALREADY\n" +
			"LANDED, which is the next sprint's inheritance either way. The escape hatch\n" +
			"is never --force; it is to leave the weave unmerged or `weave abandon` it,\n" +
			"losing one branch instead of a repo.\n\n" +
			"NO GATE IS NOT A PASS. Without --gate nothing is verified, and stop says so\n" +
			"rather than assuming; --no-verify closes anyway and records it as unverified.\n\n" +
			"It also writes what ACTUALLY happened beside what was\n" +
			"planned.\n\n" +
			"That record is the only thing that makes a cadence real. A two-hour box\n" +
			"that consistently runs four hours is not a two-hour cadence — it is a\n" +
			"four-hour cadence with a misleading label, and only the record tells you\n" +
			"which one you have. Tune the next `--for` from this, not from intent.\n\n" +
			"Stopping does not move the sprint's column: the clock and the work are\n" +
			"different questions, and a box can close with the work unfinished. That is\n" +
			"a normal outcome and worth seeing as one.",
		Example: "  bashy sprint stop 3 --gate 'go build ./... && go test ./...'\n" +
			"  bashy sprint stop 3 --gate 'make test' --note \"paused for the incident\"\n" +
			"  bashy sprint stop 3 --no-verify        # nothing to gate; on the record",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			now := time.Now().UTC()
			if gateTimeout <= 0 {
				return fmt.Errorf("gate timeout must be positive")
			}
			requireGate := !noVerify
			var rep drainReport
			return runWeaveStoryMutate(cmd, id, op, &flags, func(s *weaveStory) (string, error) {
				b := s.currentBox()
				unboxedEnd := ending && len(s.Boxes) == 0
				if b == nil && !unboxedEnd {
					// Saying "stopped" about a sprint that was never running
					// would be a small lie of exactly the kind this feature is
					// meant to remove.
					return "", fmt.Errorf("sprint #%d has no running box — `sprint start %d` opens one", id, id)
				}
				// PARK THE WORKERS FIRST. Whatever the gate says next, nothing
				// should still be writing to the tree while it is judged — a
				// gate racing a live agent measures neither.
				if b != nil && b.Draining == nil {
					b.Draining = &now
				}
				paused, problems := pauseLinkedRepos(s)
				rep.Paused = paused
				rep.Failures = problems

				// THE MINIMUM BAR: it compiles and the tests pass. A sprint
				// that stops over a regression is not parked, it is a trap —
				// the next sprint inherits damage it cannot tell from its own.
				if strings.TrimSpace(gateCmd) != "" && !flags.quietF && !flags.jsonF {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: running gate (timeout %s): %s\n", op, gateTimeout, gateCmd)
				}
				gateCtx, cancelGate := context.WithTimeout(cmd.Context(), gateTimeout)
				out := runDrainGate(gateCtx, gateDir, gateCmd)
				cancelGate()
				rep.GateRan, rep.GatePassed, rep.GateCmd = out.Ran, out.Passed, out.Command
				if b != nil {
					b.GateRan, b.GatePassed, b.GateCmd = out.Ran, out.Passed, out.Command
				}

				// CLOSING CONDITIONS: committed, pushed, pinned. A green gate
				// says the code works; it says nothing about whether the work
				// was PUT anywhere the next sprint will find it.
				var repoPath = func(run sprintRun) (string, bool) {
					dir, err := weaveQueueDirForSprintRun(run)
					if err != nil {
						return "", false
					}
					return weaveRepoRootForQueue(dir)
				}
				repos := checkClosingConditions(s, currentBoard, repoPath)
				rep.Repos = repos
				var unclean []string
				for i := range repos {
					if !repos[i].OK() {
						unclean = append(unclean, repos[i].Describe())
					}
				}

				if !force {
					if len(unclean) > 0 {
						return "", fmt.Errorf("sprint #%d NOT stopped — the repos are not wrapped up:\n  %s\n"+
							"  commit, push, and bump pins, then `sprint stop %d` again (--force records it unwrapped)",
							id, strings.Join(unclean, "\n  "), id)
					}
					if len(problems) > 0 {
						return "", fmt.Errorf("sprint #%d NOT stopped — could not park: %s\n  fix, then `sprint stop %d` again (or --force to close anyway)",
							id, strings.Join(problems, "; "), id)
					}
					if out.Ran && !out.Passed {
						// The refusal is the feature. Workers are parked, so
						// nothing is burning; the remaining job is narrow.
						tail := out.Output
						if len(tail) > 600 {
							tail = tail[len(tail)-600:]
						}
						return "", fmt.Errorf("sprint #%d NOT stopped — the gate FAILED, so this is not a good handoff state.\n"+
							"  workers are parked; fix the regression, then `sprint stop %d` again.\n"+
							"  gate: %s (exit %d)\n%s",
							id, id, out.Command, out.ExitCode, tail)
					}
					if !out.Ran && requireGate {
						return "", fmt.Errorf("sprint #%d NOT stopped — no gate given, so \"it still builds\" is unverified.\n"+
							"  pass --gate '<cmd>', or --no-verify to close it unverified on the record", id)
					}
				}

				msg := ""
				if unboxedEnd {
					msg = "ended without a recorded time-box; " + drainEvidenceSummary(&rep)
				} else {
					b.StoppedAt = &now
					elapsed := b.Elapsed(now)
					verdict := "within the box"
					if elapsed > b.Planned {
						verdict = fmt.Sprintf("OVER by %s", roundDur(elapsed-b.Planned))
					} else if d := b.Planned - elapsed; d > time.Minute {
						verdict = fmt.Sprintf("under by %s", roundDur(d))
					}
					msg = fmt.Sprintf("stopped after %s (planned %s) — %s; %s",
						roundDur(elapsed), roundDur(b.Planned), verdict, drainSummary(&rep, elapsed, b.Planned))
				}
				if strings.TrimSpace(note) != "" {
					msg += ": " + strings.TrimSpace(note)
				}
				if ending {
					// "done" must mean done. end deliberately carries no
					// --force / --no-verify, so these are HARD refusals: the
					// gate above proves the code still builds, and these two
					// prove nothing was left behind or left out. A sprint that
					// closes over an open story nobody planned, or over a dirty
					// tree, is a green result reached because nothing looked.
					if err := sprintUnansweredGate(s, "end"); err != nil {
						return "", err
					}
					if err := sprintCoverageGate(s, false, ""); err != nil {
						return "", err
					}
					if hy := sprintCheckHygiene(s); !hy.Clean() {
						return "", fmt.Errorf(
							"sprint #%d cannot end — it is not clean:\n  %s\n  `bashy sprint prune %d` for the full state and the command that fixes each",
							id, strings.Join(hy.Problems, "\n  "), id)
					}
					from := s.Column
					who := weaveStoryConductorName(s, "")
					s.Column = "done"
					_ = closeSprintRoom(s, who)
					s.Lease = nil
					msg += fmt.Sprintf("; lifecycle ended (%s → done), conductor lease released", from)
					weaveStoryAppend(s, who, "system", msg)
					return fmt.Sprintf("sprint #%d %s", id, msg), nil
				}
				weaveStoryAppend(s, weaveConductorName(""), "system", msg)
				return fmt.Sprintf("sprint #%d %s", id, msg), nil
			})
		},
	}
	if ending {
		cmd.Long = `end closes the sprint lifecycle only after it can prove a clean handoff state.

It parks every linked working agent in a resumable weave state, refuses missing
or half-allocated linked runs, verifies linked repositories are committed,
pushed, and pinned, and requires the supplied gate to pass. It then closes the
time box, moves the card to done, closes the conductor room, and releases the
lease.

There is deliberately no --force or --no-verify. Use sprint stop when the
intent is only to close the current cadence cycle.`
		cmd.Example = "  bashy sprint end 3 --gate 'go test ./...'\n" +
			"  bashy sprint end 3 --gate 'make test' --note \"release accepted\""
	}
	cmd.Flags().StringVar(&note, "note", "", "what the box actually produced")
	cmd.Flags().StringVar(&gateCmd, "gate", "", "the command proving the tree still builds and passes")
	cmd.Flags().StringVar(&gateDir, "gate-dir", "", "where to run the gate (default: cwd)")
	cmd.Flags().DurationVar(&gateTimeout, "gate-timeout", 15*time.Minute, "maximum time allowed for the gate")
	if !ending {
		cmd.Flags().BoolVar(&force, "force", false, "close even over a red gate or an unparked worker — recorded as not clean")
		cmd.Flags().BoolVar(&noVerify, "no-verify", false, "close with no gate at all, on the record as unverified")
	}
	flags.attach(cmd)
	return cmd
}

// newSprintExtendCmd lengthens a running box WITHOUT rewriting the original
// commitment.
//
// Extending exists so that the honest move — "this needs longer" — is one
// command, rather than something people route around by restarting the box and
// quietly losing the first estimate. Planned is deliberately left alone: the
// gap between what was promised and what was taken is the whole signal.
func newSprintExtendCmd() *cobra.Command {
	var flags weaveOutputFlags
	var by time.Duration
	cmd := &cobra.Command{
		Use:     "extend <sprint>",
		Short:   "Push a running sprint's cutoff back, keeping the original estimate on record",
		Example: "  bashy sprint extend 3 --by 30m",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if by <= 0 {
				return fmt.Errorf("--by must be positive (got %s)", by)
			}
			now := time.Now().UTC()
			return runWeaveStoryMutate(cmd, id, "sprint extend", &flags, func(s *weaveStory) (string, error) {
				b := s.currentBox()
				if b == nil {
					return "", fmt.Errorf("sprint #%d has no running box to extend", id)
				}
				b.Cutoff = b.Cutoff.Add(by)
				weaveStoryAppend(s, weaveConductorName(""), "system",
					fmt.Sprintf("extended by %s (original estimate %s stands)", roundDur(by), roundDur(b.Planned)))
				return fmt.Sprintf("sprint #%d extended by %s — %s", id, roundDur(by), b.Status(now)), nil
			})
		},
	}
	cmd.Flags().DurationVar(&by, "by", 30*time.Minute, "how much longer")
	flags.attach(cmd)
	return cmd
}

// newSprintStatusCmd is THE STEWARD'S VIEW: every sprint on this host at once,
// answered as a set rather than one card at a time.
//
// The kanban is organised for the person doing one thing — it groups by column,
// which is where a sprint is. A steward asks a different question, and it is
// always about the whole: what is on the clock right now, what has run past
// what it promised, and what has been left open. Those sprints are scattered
// across columns by construction, so the board can show them and still not
// answer it.
//
// It reads, and changes nothing. A steward deciding to stop or extend does that
// deliberately, per sprint; a status view that also acted would make the survey
// and the intervention the same keystroke.
func newSprintStatusCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Every sprint on this host: what is on the clock, what is over, what is idle",
		Long: "status is the steward's cross-sprint view — the whole host in one answer.\n\n" +
			"The board groups by kanban column, which is where each sprint IS. This\n" +
			"groups by clock, which is what a steward is accountable for: several\n" +
			"initiatives run at once on different cadences, and the ones needing a\n" +
			"decision are scattered across columns where no single column shows them.\n\n" +
			"It reports and changes nothing — stopping or extending stays a deliberate\n" +
			"act on a named sprint.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := flags.mode()
			dir, err := weaveStoryDir(cmd, mode, "sprint status")
			if err != nil {
				return err
			}
			q, lerr := loadWeaveQueue(dir)
			if lerr != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint status", weavecli.ExitGenericFail, lerr))
			}
			now := time.Now().UTC()

			type row struct {
				ID      int64  `json:"id"`
				Title   string `json:"title"`
				Epic    string `json:"epic,omitempty"`
				Column  string `json:"column"`
				Status  string `json:"box_status,omitempty"`
				Cycles  int    `json:"cycles,omitempty"`
				Overdue bool   `json:"overdue,omitempty"`
				Holder  string `json:"lease_holder,omitempty"`
				Contact string `json:"contact,omitempty"`
				Stale   bool   `json:"lease_stale,omitempty"`
			}
			// ONE CONDUCTOR IS ACCOUNTABLE FOR EVERY IN-PROGRESS SPRINT — that
			// is the rule `start` enforces by claiming the lease. But a lease
			// is a heartbeat with a TTL, so it can go STALE while the box is
			// still running: the conductor died (SIGKILL, token exhaustion,
			// a closed laptop) and delivery is now nobody's.
			//
			// That state is invisible if it is filed under "running" — it
			// looks exactly like healthy work. It is the steward's most
			// actionable signal, because it is the only one where nothing will
			// improve until a person acts: an overdue sprint at least has
			// someone to make the call.
			var onClock, over, idle, done, orphaned []row
			for _, s := range q.Stories {
				h, stale, free := weaveStoryLeaseState(s)
				if free {
					h = ""
				}
				r := row{ID: s.ID, Title: s.Title, Epic: s.Epic, Column: s.Column,
					Status: s.lastBox().Status(now), Cycles: len(s.Boxes), Contact: s.Contact.String(), Overdue: s.currentBox().Overdue(now), Holder: h, Stale: stale}
				switch {
				case s.currentBox().Running() && (stale || free):
					orphaned = append(orphaned, r)
				case s.currentBox().Overdue(now):
					over = append(over, r)
				case s.currentBox().Running():
					onClock = append(onClock, r)
				case len(s.Boxes) > 0:
					// Stopped, but it HAS run — a distinct state from never
					// started. A steward restarting work looks here, and a
					// sprint whose cycles keep ending short is visible only
					// from this group.
					done = append(done, r)
				case s.Column != "done":
					idle = append(idle, r)
				}
			}
			sortRows := func(rs []row) { sort.Slice(rs, func(i, j int) bool { return rs[i].ID < rs[j].ID }) }
			sortRows(onClock)
			sortRows(over)
			sortRows(idle)
			sortRows(done)
			sortRows(orphaned)

			if mode == weavecli.OutputJSON {
				return ec(emitOK(cmd.OutOrStdout(), mode, "sprint status", map[string]any{
					"now": now, "overdue": over, "on_clock": onClock, "idle": idle, "stopped": done, "unowned_delivery": orphaned,
				}))
			}

			out := cmd.OutOrStdout()
			line := func(r row) {
				lease := "  [unowned]"
				if r.Holder != "" {
					mark := "✓"
					if r.Stale {
						mark = "STALE"
					}
					lease = fmt.Sprintf("  [%s %s]", r.Holder, mark)
				}
				st := ""
				if r.Status != "" {
					st = "  " + r.Status
				}
				cyc := ""
				if r.Cycles > 1 {
					// The count is the tell that a sprint keeps being picked up
					// and put down — which a single planned-vs-actual cannot show.
					cyc = fmt.Sprintf("  ×%d", r.Cycles)
				}
				contact := ""
				if r.Contact != "" {
					contact = "  → " + r.Contact
				}
				fmt.Fprintf(out, "  #%d %s (%s)%s%s%s%s\n", r.ID, weaveTruncate(r.Title, 40), r.Column, st, cyc, lease, contact)
			}
			// SWEEP BEFORE REPORTING. A room whose holder is dead advertises a
			// channel to somebody who will never read it, and an unanswered
			// room costs more than an absent one — it consumes the time of
			// whoever trusted it. No verb can cover this case: the holder died
			// and ran nothing.
			if swept := sweepDeadRooms(q.Stories, now, currentActor()); len(swept) > 0 {
				fmt.Fprintf(out, "swept %d abandoned room(s): %s\n\n", len(swept), strings.Join(swept, ", "))
			}

			// Unowned delivery first: a running sprint with no live conductor
			// is the one state that cannot resolve itself.
			if len(orphaned) > 0 {
				fmt.Fprintf(out, "RUNNING, NO LIVE CONDUCTOR (%d) — delivery is unowned; `sprint take <id>`\n", len(orphaned))
				for _, r := range orphaned {
					line(r)
				}
				fmt.Fprintln(out)
			}
			if len(over) > 0 {
				fmt.Fprintf(out, "PAST CUTOFF (%d) — the conductor named on each decides: stop, or extend\n", len(over))
				for _, r := range over {
					line(r)
				}
				fmt.Fprintln(out)
			}
			fmt.Fprintf(out, "ON THE CLOCK (%d)\n", len(onClock))
			if len(onClock) == 0 {
				fmt.Fprintln(out, "  — nothing running; `sprint start <id> --for <dur>`")
			}
			for _, r := range onClock {
				line(r)
			}
			if len(idle) > 0 {
				fmt.Fprintf(out, "\nOPEN, NOT ON THE CLOCK (%d)\n", len(idle))
				for _, r := range idle {
					line(r)
				}
			}
			if len(done) > 0 {
				fmt.Fprintf(out, "\nSTOPPED — restartable (%d)\n", len(done))
				for _, r := range done {
					line(r)
				}
			}
			return nil
		},
	}
	flags.attach(cmd)
	return cmd
}
