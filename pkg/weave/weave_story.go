package weave

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/principal"
	"github.com/qiangli/coreutils/pkg/role"
	"github.com/qiangli/coreutils/pkg/room"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// weaveStory is the epic/sprint layer above the task queue: the DURABLE
// unit a conductor owns and hands off. Tasks (weaveItem) are its
// ephemeral workers; the sprint carries the spec, acceptance, kanban
// position, a continuity record (the resume brief a fresh conductor
// reads after an interruption), a thread, and a conductor lease. This
// is what makes conductor switches — graceful handoff OR recovery from
// a SIGKILL/token-exhaustion death — work: the successor reconstructs
// state from the sprint, never from the dead conductor's memory.
type weaveStory struct {
	ID         int64            `json:"id"`
	Title      string           `json:"title"`
	Epic       string           `json:"epic,omitempty"`        // grouping label
	SpecRef    string           `json:"spec_ref,omitempty"`    // handoff/spec doc reference
	Acceptance string           `json:"acceptance,omitempty"`  // done criteria
	Column     string           `json:"column"`                // backlog|doing|review|done
	Continuity string           `json:"continuity,omitempty"`  // the resume brief
	Owner      string           `json:"owner,omitempty"`       // durable coordination identity across pauses
	Goal       []sprintGoalItem `json:"goal,omitempty"`        // durable outcomes; completion is derived
	StoryRoots []string         `json:"story_roots,omitempty"` // repo todo stores contributing stories
	Execution  sprintExecution  `json:"execution,omitempty"`   // policy + current focus, never a copied order
	Lease      *weaveStoryLease `json:"lease,omitempty"`       // current conductor + heartbeat
	Thread     []weaveComment   `json:"thread,omitempty"`      // sprint-level history
	Runs       []sprintRun      `json:"runs,omitempty"`        // linked weave runs, CROSS-REPO
	// Boxes are the sprint's TIME CYCLES, oldest first — orthogonal to Column
	// (position) and Lease (conductor liveness). A sprint is stopped and
	// restarted freely over its life, so this is a LIST: one entry per
	// start/stop cycle, the last running if it has no StoppedAt. The history is
	// the point — see weave_story_box.go.
	Boxes []weaveStoryBox `json:"boxes,omitempty"`
	// Contact is how to reach the conductor while this sprint runs — a meet
	// room to talk in, and a bus topic to make someone notice. See
	// weave_story_contact.go.
	Contact   *sprintContact `json:"contact,omitempty"`
	Created   time.Time      `json:"created"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

// sprintRun links a sprint to a weave run (issue) in a SPECIFIC repo.
// A sprint spans multiple repos; each repo keeps its own per-repo weave
// queue, so a durable link is (opaque queue tag, id). Repo remains the human
// label; Queue prevents an old same-named checkout from changing its meaning.
type sprintRun struct {
	Repo  string `json:"repo"`
	ID    int64  `json:"id"`
	Queue string `json:"queue,omitempty"` // stable opaque <repo>-<path-hash> queue identity
	// Born is the linked run's creation time — the GENERATION discriminator.
	//
	// Queue identity names a CHECKOUT, not a run. Issue ids are queue-local
	// and RECYCLED: prune a queue and the next `weave add` reuses the freed
	// numbers. So (repo, queue, id) is not a stable name for a unit of work,
	// and a brand-new run silently inherits the identity of a retired one —
	// which made `sprint link` refuse three legitimate links across two
	// sprints, each time reporting the run as "already linked" to a sprint
	// that had long since shipped.
	//
	// Created is set once by `weave add` and never rewritten, so it separates
	// generations without a schema migration or a new id space. Empty on
	// records written before this field existed; see sameSprintRun for how
	// those are handled.
	Born time.Time `json:"born,omitempty"`
}

// weaveStoryLease is the conductor lease on a sprint. Liveness is a
// HEARTBEAT, not a PID: an LLM conductor invokes weave commands
// ephemerally (no stable process), so a lease goes stale when its
// holder stops checkpointing (death by SIGKILL / token exhaustion /
// OOM). A graceful handoff clears it; a successor takes a stale one.
//
// AttachedPID does not overturn that — it is the exception that proves it.
// The heartbeat rule is the only one available when nothing is running, and
// it stays the rule. But when a process IS holding the seat open, refusing to
// record which one throws away the single piece of evidence that can be
// rechecked at read time, and leaves a killed watch looking alive for a TTL.
type weaveStoryLease struct {
	Holder string    `json:"holder"`
	At     time.Time `json:"at"`
	// AttachedPID is set ONLY while a foreground process is holding this seat
	// open on this host — today the `sprint take/start --watch` stream. It is
	// not a second liveness model competing with the heartbeat; it is what
	// makes the heartbeat's own claim checkable in the one case where the
	// heartbeat lies.
	//
	// The lie is specific. An attached watch beats every TTL/3, so its last
	// beat is up to ten minutes old when the process dies — and a heartbeat
	// records only that somebody was alive THEN. Killed at beat+1s, the seat
	// goes on reading healthy for the rest of the TTL with nothing running,
	// which is exactly the ghost `bashy agents` reported. A holder that named
	// no process could not be caught out; one that names its own can.
	//
	// Zero is the ordinary case and means "no process claims to be holding
	// this open" — an ephemeral conductor refreshing by reading its mail
	// (RefreshSprintOwnerActivity) MUST clear it, because a one-shot read's
	// pid dies with the command and would otherwise sentence the seat to
	// death the moment the read returned.
	AttachedPID int `json:"attached_pid,omitempty"`
}

// SprintLeaseTTL is how long a conductor's heartbeat stays believable.
//
// EXPORTED because it is not private policy: `bashy agents` grades the very
// same lease when it projects the board into the live-work roster, and a
// roster ageing leases on a different clock than the board reports a conductor
// the board has already released. It carried a hand-copied literal until an
// unageable lease shipped; a constant other modules can name is the only form
// of that agreement a compiler checks.
const SprintLeaseTTL = 30 * time.Minute

var weaveStoryColumns = []string{"backlog", "doing", "review", "done"}

func isValidColumn(c string) bool {
	for _, v := range weaveStoryColumns {
		if v == c {
			return true
		}
	}
	return false
}

func findWeaveStory(q *weaveQueue, id int64) *weaveStory {
	for _, s := range q.Stories {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// weaveStoryLeaseState returns a short human marker for a sprint's lease:
// holder + fresh/STALE/free. Stale = heartbeat older than SprintLeaseTTL.
func weaveStoryLeaseState(s *weaveStory) (holder string, stale bool, free bool) {
	l := s.seat()
	switch l.Live(time.Now()) {
	case role.LivenessVacant:
		return "", false, true
	case role.LivenessLive:
		return l.Holder, false, false
	default:
		// Lapsed AND unknown both read as stale to this caller, which only has
		// two words. They are not the same thing — see role.LivenessUnknown —
		// and a caller that can act on the difference should ask seat() directly.
		return l.Holder, true, false
	}
}

// seat expresses a sprint's conductor lease as the shared occupancy type, so
// "is this held, and by whom" is answered by one rule rather than three.
//
// The TTL is this package's constant, and the VERDICT is what this function
// adds — a lease with no timestamp reads as unknown rather than as silently
// fresh, which is what a zero time compared against a TTL used to produce.
// A dead attached process is folded in here, at the one place all three
// liveness consumers already agree to ask, rather than at each of them.
func (s *weaveStory) seat() role.Seat {
	if s == nil || s.Lease == nil {
		return role.Seat{TTL: SprintLeaseTTL}
	}
	seat := role.Seat{
		Holder: s.Lease.Holder,
		// A sprint lease has one timestamp doing both jobs: it is stamped on
		// take and refreshed on checkpoint, so it is the heartbeat. Reporting
		// it as the acquisition time too would make every refresh look like a
		// new tenure.
		HeartbeatAt: s.Lease.At,
		TTL:         SprintLeaseTTL,
	}
	// A HEARTBEAT IS A CLAIM ABOUT A MOMENT; A NAMED PROCESS IS CHECKABLE NOW.
	//
	// When a foreground watch is holding this seat open it stamps its pid, and
	// it beats only every TTL/3 — so a watch killed just after a beat leaves a
	// timestamp that is still comfortably "fresh" and a seat that nothing is
	// occupying. Withdrawing the heartbeat (rather than inventing a fourth
	// liveness word the shared type does not have) lands it on UNKNOWN: held
	// per the record, with nothing saying it breathes. That is exactly what is
	// true, and it is what lets a successor take the seat instead of waiting
	// out a TTL behind a conductor that is already gone.
	if s.Lease.AttachedPID > 0 && !room.PidAlive(s.Lease.AttachedPID) {
		seat.HeartbeatAt = time.Time{}
	}
	return seat
}

// weaveConductorIdentity resolves process-local evidence for the acting
// conductor. BASHY_PRINCIPAL is the launcher's authoritative identity; turn a
// canonical URN back into the short name sprint leases store.
func weaveConductorIdentity(asFlag string) (string, bool) {
	for i, v := range []string{
		asFlag,
		os.Getenv("BASHY_PRINCIPAL"),
		os.Getenv("WEAVE_CONDUCTOR"),
		os.Getenv("BASHY_AGENT_ID"),
		os.Getenv("BASHY_AGENT"),
		os.Getenv("WEAVE_AGENT"),
	} {
		if s := strings.TrimSpace(v); s != "" {
			if i == 1 {
				if _, name, _, err := principal.ParseURN(s); err == nil {
					return name, true
				}
			}
			return s, true
		}
	}
	return "", false
}

// weaveConductorName resolves the acting conductor's name from process-local
// identity, falling back only when no launcher or command supplied one.
func weaveConductorName(asFlag string) string {
	if who, ok := weaveConductorIdentity(asFlag); ok {
		return who
	}
	return "conductor"
}

// weaveStoryConductorName bridges ephemeral acting identity with the durable
// sprint lease for authorship and lifecycle records. Ownership mutations do
// not use this fallback: start/take require their own explicit --owner.
func weaveStoryConductorName(s *weaveStory, asFlag string) string {
	if strings.TrimSpace(asFlag) != "" {
		return strings.TrimSpace(asFlag)
	}
	if s != nil && strings.TrimSpace(s.Owner) != "" {
		return s.Owner
	}
	if holder, stale, free := weaveStoryLeaseState(s); !free && !stale {
		return holder
	}
	if who, ok := weaveConductorIdentity(""); ok {
		return who
	}
	return "conductor"
}

// ---- verbs ----------------------------------------------------------

func newWeaveBoardCmd() *cobra.Command {
	var flags weaveOutputFlags
	var epic string
	cmd := &cobra.Command{
		Use:   "board",
		Short: "Show the sprint kanban (the conductor's epic/sprint board)",
		Long: `board renders the SPRINT kanban — the epic/sprint layer above the task
queue. This is what a conductor reads on pickup (NOT 'weave list', which
is the ephemeral tasks). Each sprint shows its column, conductor lease
holder (and whether the lease is STALE — the previous conductor died
without handing off), and a one-line continuity pointer.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveBoard(cmd, epic, &flags)
		},
	}
	cmd.Flags().StringVar(&epic, "epic", "", "filter to one epic")
	flags.attach(cmd)
	return cmd
}

func runWeaveBoard(cmd *cobra.Command, epic string, flags *weaveOutputFlags) error {
	mode := flags.mode()
	dir, err := weaveStoryDir(cmd, mode, "sprint board")
	if err != nil {
		return err
	}
	q, lerr := loadWeaveQueue(dir)
	if lerr != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint board", weavecli.ExitGenericFail, lerr))
	}
	stories := make([]*weaveStory, 0, len(q.Stories))
	for _, s := range q.Stories {
		if epic == "" || s.Epic == epic {
			stories = append(stories, s)
		}
	}
	sort.Slice(stories, func(i, j int) bool { return stories[i].ID < stories[j].ID })
	if mode == weavecli.OutputJSON {
		return ec(emitOK(cmd.OutOrStdout(), mode, "sprint board", map[string]any{"stories": stories}))
	}
	out := cmd.OutOrStdout()
	if len(stories) == 0 {
		fmt.Fprintln(out, "sprint board is empty — `sprint add \"<title>\" --spec <doc>`")
		return nil
	}
	now := time.Now().UTC()
	fmt.Fprintln(out, "SPRINT BOARD")
	for _, col := range weaveStoryColumns {
		fmt.Fprintf(out, "%s:\n", col)
		any := false
		for _, s := range stories {
			if s.Column != col {
				continue
			}
			any = true
			lease := ""
			if h, stale, free := weaveStoryLeaseState(s); !free {
				mark := "✓"
				if stale {
					mark = "STALE"
				}
				lease = fmt.Sprintf("  [%s %s]", h, mark)
			}
			epicTag := ""
			if s.Epic != "" {
				epicTag = "(" + s.Epic + ") "
			}
			// The box rides on the same row as the lease, because a
			// conductor scanning several concurrent sprints needs "which of
			// these is out of time" answerable at a glance rather than one
			// `sprint show` at a time.
			box := ""
			if st := s.lastBox().Status(now); st != "" {
				box = "  [" + st + "]"
			}
			// An OPEN sprint nobody can reach is the one row a scan must not
			// slide past: the board is where a conductor decides what to pick
			// up, and a card advertising an owner that answers nothing looks
			// exactly like a healthy one.
			reach := ""
			if p := sprintCheckReachability(s).Problems; len(p) > 0 {
				reach = fmt.Sprintf("  [UNREACHABLE: %d]", len(p))
			}
			fmt.Fprintf(out, "  #%d %s%s%s%s%s\n", s.ID, epicTag, weaveTruncate(s.Title, 52), lease, box, reach)
		}
		if !any {
			fmt.Fprintln(out, "  —")
		}
	}
	// The kanban scatters running sprints across columns, but "what is on the
	// clock right now, and is any of it over" is a question about the set —
	// which is exactly the question a cadence asks, and the reason several
	// simultaneous boxes stay manageable.
	var running, overdue []string
	for _, s := range stories {
		if !s.currentBox().Running() {
			continue
		}
		running = append(running, fmt.Sprintf("#%d %s", s.ID, s.currentBox().Status(now)))
		if s.currentBox().Overdue(now) {
			overdue = append(overdue, fmt.Sprintf("#%d", s.ID))
		}
	}
	if len(running) > 0 {
		fmt.Fprintf(out, "\non the clock: %s\n", strings.Join(running, " · "))
		if len(overdue) > 0 {
			fmt.Fprintf(out, "  %s past cutoff — `sprint stop <id>` or `sprint extend <id> --by <dur>`\n",
				strings.Join(overdue, ", "))
		}
	}
	return nil
}

// NewSprintCmd is the PLAN/HANDOFF surface — peer to `bashy weave`. A
// SPRINT is one initiative that spans multiple repos; its board is
// USER-GLOBAL. `bashy weave` is the per-repo EXECUTION engine for the
// runs a sprint links. `bashy sprint` with no subcommand shows the board.
func NewSprintCmd() *cobra.Command {
	var flags weaveOutputFlags
	var epic string
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "Plan/handoff: the cross-repo sprint kanban above weave's per-repo runs",
		Long: `sprint is the conductor's PLAN/HANDOFF layer — the cross-repo kanban
above weave. A sprint is one initiative that spans multiple repos; its
runs are executed per-repo by ` + "`bashy weave`" + `. The board is user-global;
` + "`bashy sprint`" + ` (no subcommand) shows it.

Each sprint card carries a spec-ref, acceptance, a kanban column, a
CONTINUITY record (the resume brief), a conductor LEASE, and cross-repo
run links {repo, id}. Durability (survive Ctrl+C / SIGKILL / token
exhaustion): checkpoint often. The common lifecycle is start → pause →
resume → end; handoff/take/stop remain the lower-level compatibility verbs.

TRACKING: create and link a sprint story before implementation begins. Every
delivery commit must end with one Sprint trailer and one Story/Story-ID pair
for each story it advances:

  Sprint: #87
  Story: #110
  Story-ID: d1e86f29d7a7

Install the local fail-closed guard with ` + "`bashy sprint hooks install`" + `.
The subject remains a normal conventional-commit summary; the trailers are
the authoritative trace from delivered code back to sprint work.

MANAGEMENT: the sprint owner is responsible for delivery from start to end,
keeps priority and progress current, receives the inbox automatically under the
sprint's recorded owner name, and coordinates before touching another owner's work.
Never delete or destroy work owned by another agent.

When taking over a sprint, use its recorded owner name for message-board and
meet coordination. To use another name, update sprint ownership first.
Proactively review and merge delegated changes, then clean up only the
branches, worktrees, and weave workspaces owned by this sprint.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveBoard(cmd, epic, &flags) // default = board
		},
	}
	// Drop cobra's auto `completion` subcommand — sprint is an agentic
	// surface, not an interactive human shell (mirrors NewWeaveCmd).
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.Flags().StringVar(&epic, "epic", "", "filter to one epic")
	flags.attach(cmd)

	// SELF-REPORTING STRUCTURAL ERRORS — the same two installs NewWeaveCmd
	// does (weave.go), and for the same reason. flags.attach above sets
	// SilenceErrors/SilenceUsage so a subverb's own envelope is never
	// double-printed; that silence ALSO swallows the errors cobra raises
	// before any RunE runs. sprint had the silence and neither reporter, so
	// every subverb exited 1 having written zero bytes to stdout AND stderr:
	//
	//	sprint checkpoint 87 --as x   exit=1  out=0B  err=0B
	//	sprint comment 87 --body x    exit=1  out=0B  err=0B
	//	sprint show 87 --nope         exit=1  out=0B  err=0B
	//
	// A silent exit 1 is indistinguishable from "ran, found nothing". It cost
	// three recorded incidents on sprint #87 alone — most recently a
	// continuity record its conductor believed was written and was not,
	// because `--as` is not a checkpoint flag and nothing said so.
	//
	// cobra's FlagErrorFunc() climbs to the parent, so one install covers
	// every subverb; installArgsErrorReporting has no such inheritance and
	// walks the tree, so it must run AFTER AddCommand below.
	cmd.SetFlagErrorFunc(weaveFlagErrorFunc)
	cmd.AddCommand(
		newWeaveBoardCmd(),
		newWeaveStoryAddCmd(),
		newWeaveStoryShowCmd(),
		newWeaveStoryMoveCmd(),
		newWeaveStoryPruneCmd(),
		newSprintClaimCmd(),
		newSprintYieldCmd(),
		newSprintSubmitCmd(),
		newWeaveStoryEditCmd(),
		newWeaveStoryRmCmd(),
		newSprintStatusCmd(),
		newSprintWhoCmd(),
		newSprintPingCmd(),
		newSprintStartCmd(),
		newSprintInstructCmd(),
		newSprintStopCmd(),
		newSprintEndCmd(),
		newSprintExtendCmd(),
		newWeaveStoryTakeCmd(),
		newWeaveStoryHandoffCmd(),
		newSprintAbortCmd(),
		newWeaveStoryCommentCmd(),
		newWeaveStoryLinkCmd(),
		newWeaveStoryUnlinkCmd(),
		newWeaveCheckpointCmd(),
		newSprintGoalCmd(),
		newSprintTrackCmd(),
		newSprintNextCmd(),
		newSprintFocusCmd(),
		newSprintCommitMsgCmd(),
		newSprintHooksCmd(),
		// Conductor-coordination, moved here from `weave` (plan layer,
		// not per-repo execution): the cloudbox shared-session group and
		// the conductor director.
		newSprintSessionCmd(),
		newWeaveConductCmd(),
	)

	// Positional-argument half. Must follow AddCommand: it walks
	// root.Commands() to wrap each subverb's Args validator, so a
	// subcommand added afterwards would not be covered.
	installArgsErrorReporting(cmd)
	return cmd
}

// newSprintSessionCmd groups the Cloudbox shared-session verbs (live
// multi-host collaboration on a sprint) under `sprint session`. They
// were scattered at weave's top level, where `take`/`handoff` collided
// with sprint's own board lease verbs; nesting them disambiguates
// (`sprint take` = local board lease, `sprint session take` = shared
// cloudbox session lease) and keeps `weave` execution-only.
func newSprintSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Cloudbox shared sessions (live multi-host collaboration on a sprint)",
	}
	cmd.AddCommand(
		newWeaveSessionsCmd(), // `session list`
		newWeaveJoinCmd(),
		newWeaveTakeCmd(),
		newWeaveHandoffCmd(),
		newWeaveNoteCmd(),
		newWeaveSteerCmd(),
		newWeaveRosterCmd(),
		newWeaveShareCmd(),
		newWeaveSharesCmd(),
		newWeaveUnshareCmd(),
	)
	return cmd
}

func newWeaveStoryAddCmd() *cobra.Command {
	var flags weaveOutputFlags
	var epic, spec, acceptance, column string
	cmd := &cobra.Command{
		Use:   `add "<title>"`,
		Short: "Create a sprint card on the board",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if column == "" {
				column = "backlog"
			}
			if !isValidColumn(column) {
				return fmt.Errorf("column must be one of %s", strings.Join(weaveStoryColumns, "|"))
			}
			return runWeaveStoryAdd(cmd, strings.Join(args, " "), epic, spec, acceptance, column, &flags)
		},
	}
	cmd.Flags().StringVar(&epic, "epic", "", "epic grouping label")
	cmd.Flags().StringVar(&spec, "spec", "", "spec/handoff doc reference (e.g. docs/p3-handoff.md)")
	cmd.Flags().StringVar(&acceptance, "acceptance", "", "acceptance / done criteria")
	cmd.Flags().StringVar(&column, "column", "backlog", strings.Join(weaveStoryColumns, "|"))
	flags.attach(cmd)
	return cmd
}

func runWeaveStoryAdd(cmd *cobra.Command, title, epic, spec, acceptance, column string, flags *weaveOutputFlags) error {
	mode := flags.mode()
	dir, err := weaveStoryDir(cmd, mode, "sprint add")
	if err != nil {
		return err
	}
	var newID int64
	lockErr := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		if q.NextStoryID == 0 {
			q.NextStoryID = 1
		}
		newID = q.NextStoryID
		q.NextStoryID++
		s := &weaveStory{
			ID: newID, Title: title, Epic: epic, SpecRef: spec,
			Acceptance: acceptance, Column: column,
			Execution: sprintExecution{PriorityFirst: true},
			Created:   time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		weaveStoryAppend(s, "conductor", "system", fmt.Sprintf("created in %s", column))
		q.Stories = append(q.Stories, s)
		return nil
	})
	if lockErr != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint add", weavecli.ExitGenericFail, lockErr))
	}
	if mode == weavecli.OutputJSON {
		return ec(emitOK(cmd.OutOrStdout(), mode, "sprint add", map[string]any{"sprint": newID, "column": column}))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "sprint add: sprint #%d in %s\n", newID, column)
	return nil
}

func newWeaveStoryShowCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{
		Use:   "show <sprint>",
		Short: "Show a sprint card: spec, acceptance, continuity, lease, thread, tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			return runWeaveStoryShow(cmd, id, &flags)
		},
	}
	flags.attach(cmd)
	return cmd
}

func runWeaveStoryShow(cmd *cobra.Command, id int64, flags *weaveOutputFlags) error {
	mode := flags.mode()
	dir, err := weaveStoryDir(cmd, mode, "sprint show")
	if err != nil {
		return err
	}
	q, lerr := loadWeaveQueue(dir)
	if lerr != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint show", weavecli.ExitGenericFail, lerr))
	}
	s := findWeaveStory(q, id)
	if s == nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint show", weavecli.ExitInvalidArg,
			fmt.Errorf("sprint #%d not found", id)))
	}
	if mode == weavecli.OutputJSON {
		progress := make([]map[string]any, 0, len(s.Goal))
		for _, g := range s.Goal {
			progress = append(progress, map[string]any{"id": g.ID, "checked": sprintGoalDone(g), "dangling": sprintGoalDangling(g)})
		}
		next, nerr := nextSprintStory(s)
		if nerr != nil {
			return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint show", weavecli.ExitGenericFail, nerr))
		}
		return ec(emitOK(cmd.OutOrStdout(), mode, "sprint show", map[string]any{
			"sprint": s, "goal_progress": progress, "next_story": next,
		}))
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "sprint #%d [%s] — %s\n", s.ID, s.Column, s.Title)
	if s.Epic != "" {
		fmt.Fprintf(out, "  epic:       %s\n", s.Epic)
	}
	if s.SpecRef != "" {
		fmt.Fprintf(out, "  spec:       %s\n", s.SpecRef)
	}
	if s.Acceptance != "" {
		fmt.Fprintf(out, "  acceptance: %s\n", s.Acceptance)
	}
	renderSprintExecution(out, s)
	// The plan's blind spot and the sprint's reachability belong HERE, next to
	// the checklist they qualify. `sprint show` is what a conductor reads on
	// resume; a plan that has stopped describing the sprint, or an owner nobody
	// can reach, are exactly the facts that must not wait for a close attempt.
	renderSprintCoverage(out, s)
	renderSprintReachability(out, s)
	if h, stale, free := weaveStoryLeaseState(s); !free {
		st := "fresh"
		if stale {
			st = fmt.Sprintf("STALE (no heartbeat for %s — take it)", time.Since(s.Lease.At).Round(time.Minute))
		}
		fmt.Fprintf(out, "  conductor:  %s (%s)\n", h, st)
	} else if s.Owner != "" {
		fmt.Fprintf(out, "  conductor:  %s (owner; lease unclaimed — `sprint resume %d`)\n", s.Owner, s.ID)
	} else {
		fmt.Fprintf(out, "  conductor:  (unclaimed — `sprint take %d`)\n", s.ID)
	}
	if len(s.Runs) > 0 {
		strs := make([]string, len(s.Runs))
		for i, r := range s.Runs {
			strs[i] = fmt.Sprintf("%s#%d", r.Repo, r.ID)
		}
		fmt.Fprintf(out, "  runs:       %s\n", strings.Join(strs, " "))
	}
	fmt.Fprintln(out, "  ── continuity (resume brief) ──")
	if strings.TrimSpace(s.Continuity) == "" {
		fmt.Fprintln(out, "  (none yet — conductor: `sprint checkpoint` after each step)")
	} else {
		for _, line := range strings.Split(s.Continuity, "\n") {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
	if len(s.Thread) > 0 {
		fmt.Fprintln(out, "  ── thread ──")
		for _, c := range s.Thread {
			fmt.Fprintf(out, "  [%s] %s (%s): %s\n", c.At.Format("01-02 15:04"), c.Author, c.Kind, c.Body)
		}
	}
	return nil
}

func newWeaveStoryMoveCmd() *cobra.Command {
	var flags weaveOutputFlags
	var force bool
	var forceReason string
	cmd := &cobra.Command{
		Use:   "move <sprint> <backlog|doing|review|done>",
		Short: "Move a sprint to a kanban column",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			col := strings.ToLower(strings.TrimSpace(args[1]))
			if !isValidColumn(col) {
				return fmt.Errorf("column must be one of %s", strings.Join(weaveStoryColumns, "|"))
			}
			return runWeaveStoryMutate(cmd, id, "sprint move", &flags, func(s *weaveStory) (string, error) {
				if col == "done" {
					if remaining := sprintUncheckedGoals(s); len(remaining) > 0 {
						return "", fmt.Errorf("sprint #%d has unchecked goal items: %s", id, strings.Join(remaining, ", "))
					}
					// The goal check above asks whether the PLAN is finished.
					// This asks whether the WORK is — a story filed into the
					// sprint and never linked to a goal item was invisible to
					// the first check, so a sprint could close clean over open
					// work purely because nothing looked at it.
					if err := sprintCoverageGate(s, force, forceReason); err != nil {
						return "", err
					}
					if hy := sprintCheckHygiene(s); !hy.Clean() && !force {
						return "", fmt.Errorf("sprint #%d is not clean:\n  %s\n  run `bashy sprint prune %d` for the full state, or --force --reason \"<why>\"",
							id, strings.Join(hy.Problems, "\n  "), id)
					}
				}
				from := s.Column
				s.Column = col
				// Leaving an open column ends the sprint's need for a room. This
				// is the counterpart of retaining it across pause/handoff: the
				// room's lifetime is the OPEN CARD's, so it closes here and not
				// when a conductor happens to step away.
				if sprintColumnOpen(from) && !sprintColumnOpen(col) {
					_ = closeSprintRoom(s, weaveStoryConductorName(s, ""))
				}
				weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system", fmt.Sprintf("moved %s → %s", from, col))
				return fmt.Sprintf("sprint #%d %s → %s", id, from, col), nil
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "close over an unclean or uncovered state (requires --reason)")
	cmd.Flags().StringVar(&forceReason, "reason", "", "why closing over open findings is correct — recorded on the card")
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryTakeCmd() *cobra.Command {
	var flags weaveOutputFlags
	var as string
	var force bool
	cmd := &cobra.Command{
		Use:     "take <sprint>",
		Aliases: []string{"resume"},
		Short:   "Claim the conductor lease on a sprint (takes over a STALE/dead conductor)",
		Long: `take claims the conductor lease so a new conductor can pick up a sprint —
the human-directed switch, or recovery after the previous conductor died
(SIGKILL / token exhaustion). A free or STALE lease is taken directly; a
FRESH lease requires --force (and is recorded). After taking, read the
continuity record (sprint show) and resume.

YOU ARE NOW THE SPRINT MANAGER, NOT ITS SOLE WORKER.

THIS IS A JUDGEMENT JOB, NOT A DISPATCH LOOP. The objectives below COMPETE:
delivery speed wants more agents, cost wants fewer, the sprint GOAL wants the
right ones, and capacity decides what is even possible right now. No single rule
wins — optimize one and ignore the rest and you deliver late, or over budget, or
the wrong thing well. Weigh them per story, record the call on the sprint thread
whenever it is not obvious, and revisit as the backlog and the fleet change.

Taking a sprint makes you responsible for DELIVERY, and the fastest delivery
uses the whole fleet. So the default is: prioritize the stories, then DELEGATE
them to agents from ` + "`bashy agents list`" + `, matching each story to a capable
agent and running independent stories in parallel. Working through a sprint
alone is the EXCEPTION, justified only for a story short enough to finish
immediately and needing no other agent.

THE ROSTER IS YOURS TO CHANGE, NOT A FIXED MENU. ` + "`bashy agents list`" + ` is a
living roster the manager MAINTAINS: register a new binding with
` + "`bashy agents add`" + `, branch one for a single task with ` + "`bashy agents clone`" + `
(` + "`--ephemeral --task`" + ` so it is reaped when the work closes), and adjust an
entry when the work needs something the fleet does not yet have. Matching the
fleet to the backlog is part of managing the sprint, not a request to escalate —
a manager who leaves a story unstarted because no listed agent fits has stopped
one step short of the job.

BUT MORE AGENTS IS NOT AUTOMATICALLY FASTER, AND IT IS NEVER FREE. Parallelism
pays only across INDEPENDENT stories; dependent ones serialize no matter how many
agents hold them, and a worker started on blocked work burns tokens to wait.
Every agent also spends a real budget and contends for the same seats and rate
limits — a flat-quota model that runs out BLOCKS, and a flat-then-metered one
keeps working and starts charging, which is the silent one. Check
` + "`bashy weave fleet`" + ` for who is actually available (installed, signed in,
not cooling down) before widening, and widen to the number of ready independent
stories rather than to the size of the roster.

MATCH THE AGENT TO THE STORY, AND PREFER THE ONE THAT DOES NOT METER. Two
routing rules, both ordinary service economics rather than anything specific to
this fleet:

  - Capability to difficulty. A band is a peg (` + "`bashy agents list --min-band`" + `);
    spending an L4 on a mechanical story buys nothing and consumes the seat the
    hard story will need. Send the easy work to the cheaper, smaller agent.
  - Billing before auth. Prefer FLAT-BILLED agents over metered ones, and read
    ` + "`Model.Billing`" + `, never ` + "`Model.Kind`" + `: the two are orthogonal on purpose,
    because a plan can be flat-rate and still authenticate with an API key. An
    agent that looks metered by its auth may be prepaid, and routing on the
    wrong field sends work to the one that bills per token.

An idle fleet beside a queued backlog is the failure this seat exists to
prevent; a crowd of agents on one serialized story is the opposite failure and
costs more. Delegation transfers execution; it never transfers accountability —
you still gate, converge and report.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			return runSprintOwnerLifecycle(cmd, &flags, id, "sprint take", "transfer managed sprint owner", func() error {
				if !cmd.Flags().Changed("owner") || strings.TrimSpace(as) == "" {
					return fmt.Errorf("--owner is required: choose a project manager NAME from `bashy agents list`; the calling agent must ask the user rather than guess")
				}
				who := strings.TrimSpace(as)
				if err := validateSprintClaimant(who); err != nil {
					return err
				}
				who, _ = canonicalFleetAgentName(who)
				before, err := sprintOwnerSnapshot(id)
				if err != nil {
					return err
				}
				prev, stale, free := weaveStoryLeaseState(before)
				if !free && !stale && prev != who && !force {
					return fmt.Errorf("sprint #%d lease is held by %s (fresh) — coordinate, or --force to take over", id, prev)
				}
				expectedOwner := strings.TrimSpace(before.Owner)
				if expectedOwner != "" && !strings.EqualFold(expectedOwner, who) {
					cwd, _ := os.Getwd()
					if err := retireSprintOwnerSession(cmd.Context(), id, expectedOwner, cwd); err != nil {
						return fmt.Errorf("cannot transfer sprint #%d manager from %s to %s: %w", id, expectedOwner, who, err)
					}
				}
				return runWeaveStoryMutate(cmd, id, "sprint take", &flags, func(s *weaveStory) (string, error) {
					if strings.TrimSpace(s.Owner) != expectedOwner {
						return "", fmt.Errorf("sprint #%d project manager changed concurrently from %s to %s", id, expectedOwner, s.Owner)
					}
					prev, stale, free := weaveStoryLeaseState(s)
					if !free && !stale && prev != who && !force {
						return "", fmt.Errorf("sprint #%d lease is held by %s (fresh) — coordinate, or --force to take over", id, prev)
					}
					// The room is the SPRINT's, so a takeover inherits it rather
					// than replacing it — the transcript left by the previous
					// conductor is the handover context, and closing it to open an
					// identical one discarded that and filed a set of meet minutes
					// per hop. Only a sprint with no room gets a new one.
					if s.currentBox().Running() || sprintColumnOpen(s.Column) {
						ensureSprintRoom(s, who)
					}
					// AN OWNER MUST BE AN ADDRESS. Refuse a name that resolves to
					// no agent: every coordination surface (mb, chat, inbox, ping)
					// keys on this string, and one that names nobody is worse than
					// none — it is printed as a contact and silently never answers.
					// CLAIM-TIME: the seat must be RUNNING, not merely declared.
					// A sprint seated to a name with no process behind it accepts
					// room messages and inbox mail that nobody will ever read.
					s.Lease = &weaveStoryLease{Holder: who, At: time.Now().UTC()}
					s.Owner = who
					// The brief is printed on TAKE, not offered by a separate verb.
					// `resume` existed only to show it, which meant the takeover
					// path that matters most — inheriting from a conductor that
					// DIED — was the one that printed nothing.
					brief := strings.TrimSpace(s.Continuity)
					if brief == "" {
						brief = "(no continuity brief recorded)"
					}
					switch {
					case free:
						weaveStoryAppend(s, who, "system", "took conductor lease (was unclaimed)")
					case stale:
						weaveStoryAppend(s, who, "system", fmt.Sprintf("took STALE conductor lease from %s (recovery)", prev))
					case prev == who:
						weaveStoryAppend(s, who, "system", "resumed own conductor lease and delivery stream")
					default:
						weaveStoryAppend(s, who, "system", fmt.Sprintf("force-took conductor lease from %s", prev))
					}
					return fmt.Sprintf("sprint #%d: %s is now conductor — use this exact name for mb/Meet/chat/ping; %s\ncontinuity: %s", id, who, sprintReadyLine(id, who), brief), nil
				})
			})
		},
	}
	// Same canon as `sprint start`: --owner is the spelling and PROJECT MANAGER
	// is what the sprint calls its owner.
	role.AttachOwner(cmd.Flags(), &as, role.ProjectManager,
		"accountable for delivery from start to end; required explicitly on every take")
	cmd.Flags().BoolVar(&force, "force", false, "take over a fresh lease")
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryHandoffCmd() *cobra.Command {
	var flags weaveOutputFlags
	var message string
	cmd := &cobra.Command{
		Use:     "handoff <sprint>",
		Aliases: []string{"pause"},
		Short:   "Graceful handoff: checkpoint continuity + release the conductor lease",
		Long: `handoff is the conductor exit — Ctrl+C, a planned switch, low context,
or simply stepping away: record a continuity brief and RELEASE the lease so
whoever comes next starts from it. Aliased as pause, which it absorbed.

There is deliberately ONE exit verb. "I am coming back" and "somebody else
takes this" looked like different acts, but which one a departure turns out to
be is not knowable when you leave: a conductor that pauses for five minutes may
be killed, and its successor inherits a sprint with no brief. So the brief is
always required, and the sprint stays equally resumable by anyone.

Two things are untouched. Linked weave workers keep running — changing who
conducts must not stop the work. And the room stays open while the card is
open, so the interval with no conductor is still reachable.

This releases the SEAT, not the CLOCK: the time box keeps running. Use stop to
close the cadence, end when the sprint is finished.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			message = strings.TrimSpace(message)
			if message == "" {
				return fmt.Errorf("-m <continuity brief> required: whoever comes next starts from it, " +
					"and whether that is you or a stranger is not knowable here")
			}
			return runWeaveStoryMutate(cmd, id, "sprint handoff", &flags, func(s *weaveStory) (string, error) {
				// A handoff releases the ROLE; it does not end the SPRINT.
				// "The successor opens their own" was true only once the
				// successor arrived — and the interval before that is the whole
				// problem: an open card, a running box, and no room during the
				// exact window an arriving agent needs to ask who is taking it.
				// The room now belongs to the sprint and waits for them.
				who := weaveStoryConductorName(s, "")
				if err := sprintUnansweredGate(s, "hand off"); err != nil {
					return "", err
				}
				roomNote := "; room stays open for the successor"
				if !sprintRoomRetained(s) {
					_ = closeSprintRoom(s, who)
					roomNote = ""
				}
				// THE BRIEF IS ALWAYS REQUIRED. `pause` demanded one and
				// `handoff` did not, on the theory that a pause is temporary
				// and a handoff is not — but which of the two this turns out to
				// be is NOT KNOWABLE HERE. A conductor stepping away for five
				// minutes may be SIGKILLed, and its "I will be back" becomes a
				// stranger's cold start with no brief. That is why the lease is
				// a heartbeat; the brief follows the same reasoning.
				s.Continuity = message + sprintStateAddendum(s)
				weaveStoryAppend(s, who, "system", "handed off — released conductor lease"+roomNote)
				s.Lease = nil
				return fmt.Sprintf("sprint #%d: lease released; continuity recorded for the next conductor", id), nil
			})
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "continuity brief for whoever comes next (required)")
	flags.attach(cmd)
	return cmd
}

func newSprintAbortCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{
		Use:   "abort <sprint>",
		Short: "Emergency stop: kill every linked run, clear the conductor lease, park the sprint",
		Long: `abort is the hard stop for a sprint gone wrong (a runaway or misdirected
conductor). It KILLS every linked weave run (stops the agent wrapper,
preserves the workspace — run weave prune later), clears the conductor
lease, and parks the sprint in backlog. Unlike handoff (graceful — leaves
in-flight runs intact for a successor) abort tears the work down.

A Bashy-managed project-manager session is stopped before the board is parked;
abort refuses rather than record a completed hard stop while that managed agent
is still running. External managers remain the caller's responsibility.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			who := weaveConductorName("")
			return runSprintOwnerLifecycle(cmd, &flags, id, "sprint abort", "abort managed sprint owner", func() error {
				before, err := sprintOwnerSnapshot(id)
				if err != nil {
					return err
				}
				cwd, _ := os.Getwd()
				if err := stopRecordedSprintOwner(cmd.Context(), before, cwd); err != nil {
					return fmt.Errorf("cannot abort sprint #%d while manager %s is still running: %w", id, before.Owner, err)
				}
				return runWeaveStoryMutate(cmd, id, "sprint abort", &flags, func(s *weaveStory) (string, error) {
					if strings.TrimSpace(s.Owner) != strings.TrimSpace(before.Owner) {
						return "", fmt.Errorf("sprint #%d project manager changed concurrently from %s to %s", id, before.Owner, s.Owner)
					}
					var killed, failed []string
					for _, r := range s.Runs {
						qd, derr := weaveQueueDirForSprintRun(r)
						if derr != nil {
							failed = append(failed, fmt.Sprintf("%s#%d (%v)", r.Repo, r.ID, derr))
							continue
						}
						if kerr := applyDirectiveKill(qd, r.ID, "sprint abort"); kerr != nil {
							failed = append(failed, fmt.Sprintf("%s#%d (%v)", r.Repo, r.ID, kerr))
							continue
						}
						killed = append(killed, fmt.Sprintf("%s#%d", r.Repo, r.ID))
					}
					s.Lease = nil
					s.Column = "backlog"
					weaveStoryAppend(s, who, "system", fmt.Sprintf("ABORTED — killed %d run(s), cleared lease, parked in backlog", len(killed)))
					msg := fmt.Sprintf("sprint #%d ABORTED — killed [%s]; lease cleared; parked in backlog", id, strings.Join(killed, " "))
					if len(s.Runs) == 0 {
						msg = fmt.Sprintf("sprint #%d ABORTED — no linked runs; lease cleared; parked in backlog", id)
					}
					if len(failed) > 0 {
						msg += "\n  could not kill (do it manually): " + strings.Join(failed, ", ")
					}
					return msg, nil
				})
			})
		},
	}
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryCommentCmd() *cobra.Command {
	var flags weaveOutputFlags
	var kind, author, message string
	cmd := &cobra.Command{
		Use:   `comment <sprint> ["<text>"]`,
		Short: "Append to a sprint's history thread",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			body := message
			if body == "" {
				body = strings.Join(args[1:], " ")
			}
			if strings.TrimSpace(body) == "" {
				return fmt.Errorf("text required (positional or -m)")
			}
			who := author
			if who == "" {
				who = weaveConductorName("")
			}
			return runWeaveStoryMutate(cmd, id, "sprint comment", &flags, func(s *weaveStory) (string, error) {
				weaveStoryAppend(s, who, kind, body)
				return fmt.Sprintf("sprint #%d +comment", id), nil
			})
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "note", "note|progress|decision|review|blocker")
	cmd.Flags().StringVar(&author, "author", "", "default $WEAVE_CONDUCTOR/$WEAVE_AGENT")
	cmd.Flags().StringVarP(&message, "message", "m", "", "comment text")
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryLinkCmd() *cobra.Command {
	var flags weaveOutputFlags
	var task int64
	var repo, queue string
	cmd := &cobra.Command{
		Use:   "link <sprint> --repo <name> --task <issue>",
		Short: "Link a pointed weave run (repo + issue) to a sprint — runs are cross-repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if task <= 0 || strings.TrimSpace(repo) == "" {
				return fmt.Errorf("--repo <name> and --task <issue> required (a run lives in a specific repo)")
			}
			// Validate the referenced run BEFORE taking the sprint mutation lock.
			// A sprint link promises that the estimate governs execution; linking
			// missing, unpointed, or corrupt work would make that promise false.
			linked, err := resolveAndValidateSprintRunLink(repo, task, queue)
			if err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), flags.mode(), "sprint link",
					weavecli.ExitInvalidArg, err))
			}
			return runWeaveStoryMutate(cmd, id, "sprint link", &flags, func(s *weaveStory) (string, error) {
				for _, story := range currentBoard {
					// A DONE sprint's links are a historical record, not a live
					// claim. Blocking on them means a run can never be worked
					// again once any sprint that touched it has shipped — and
					// with recycled ids it also means a NEW run inherits the
					// claim of a retired one. A run may be claimed by at most
					// one sprint that is still in play; `sprint show` keeps the
					// finished sprint's record either way.
					if story.ID != id && story.Column == "done" {
						continue
					}
					for _, r := range story.Runs {
						same, serr := sameSprintRun(r, linked)
						if serr != nil {
							return "", fmt.Errorf("cannot prove %s#%d is not already linked: %w", repo, task, serr)
						}
						if !same {
							continue
						}
						if story.ID == id {
							return fmt.Sprintf("sprint #%d already links %s#%d", id, repo, task), nil
						}
						return "", fmt.Errorf("%s#%d is already linked to sprint #%d", repo, task, story.ID)
					}
				}
				s.Runs = append(s.Runs, linked)
				return fmt.Sprintf("sprint #%d linked %s#%d", id, repo, task), nil
			})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo the run lives in")
	cmd.Flags().StringVar(&queue, "queue", "", "exact opaque queue tag when multiple same-named checkouts contain the run")
	cmd.Flags().Int64Var(&task, "task", 0, "pointed weave run/issue id in that repo (points must be 1,2,3,5,8)")
	flags.attach(cmd)
	return cmd
}

// sprintRunMatchesRef reports whether a linked run record names the run the
// caller asked to unlink.
//
// This deliberately does NOT go through sameSprintRun. That helper resolves a
// legacy record's queue directory on disk to decide identity, and returns an
// error when it cannot — correct for `link`, where creating a duplicate claim
// is the hazard, but exactly wrong here. The reason a link needs removing is
// usually that the run is GONE: abandoned, pruned, or its whole queue deleted.
// An unlink that fails once the referent disappears can never clean up the one
// case it exists for, so matching is decided from the record alone.
//
// An explicit --queue must match exactly. A record predating queue identity
// carries an empty Queue and therefore never matches an explicit one; omit the
// flag to reach those.
func sprintRunMatchesRef(r sprintRun, repo string, task int64, queue string) bool {
	if r.Repo != repo || r.ID != task {
		return false
	}
	if queue != "" {
		return r.Queue == queue
	}
	return true
}

func newWeaveStoryUnlinkCmd() *cobra.Command {
	var flags weaveOutputFlags
	var task int64
	var repo, queue string
	cmd := &cobra.Command{
		Use:   "unlink <sprint> --repo <name> --task <issue>",
		Short: "Remove a linked weave run from a sprint — the inverse of `sprint link`",
		Long: `Detach a run from a sprint card.

Runs are linked to record which work a sprint governs. When a run is abandoned,
pruned, or re-filed under a new id, its link outlives it and the card keeps
naming work that no longer exists. Without a way to remove one, a card's run
list can only ever grow, and a long campaign accumulates dead references that
every later reader has to chase before discovering they lead nowhere.

Unlink does NOT require the run to still exist — a dangling reference is the
usual reason to run it. It removes the card's claim and nothing else: the run
itself, if it is still there, is untouched and may be linked to another sprint
afterwards.

Ambiguity is refused rather than guessed. If a card links several runs that
share a repo and id across different checkouts, name the one you mean with
--queue instead of removing whichever happened to be stored first.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if task <= 0 || strings.TrimSpace(repo) == "" {
				return fmt.Errorf("--repo <name> and --task <issue> required (a run lives in a specific repo)")
			}
			return runWeaveStoryMutate(cmd, id, "sprint unlink", &flags, func(s *weaveStory) (string, error) {
				var matched []int
				for i, r := range s.Runs {
					if sprintRunMatchesRef(r, repo, task, queue) {
						matched = append(matched, i)
					}
				}
				if len(matched) == 0 {
					// Report rather than fail: the caller's intent is "this
					// card must not link that run", and that is already true.
					// The wording must not read as though something was
					// removed — a no-op reported as a removal is how a stale
					// link survives a cleanup that believed it succeeded.
					return fmt.Sprintf("sprint #%d does not link %s#%d — nothing removed", id, repo, task), nil
				}
				if len(matched) > 1 {
					qs := make([]string, 0, len(matched))
					for _, i := range matched {
						q := s.Runs[i].Queue
						if q == "" {
							q = "(no queue recorded)"
						}
						qs = append(qs, q)
					}
					return "", fmt.Errorf("sprint #%d links %s#%d %d times across queues %s: name one with --queue",
						id, repo, task, len(matched), strings.Join(qs, ", "))
				}
				i := matched[0]
				removed := s.Runs[i]
				s.Runs = append(s.Runs[:i], s.Runs[i+1:]...)
				if removed.Queue != "" {
					return fmt.Sprintf("sprint #%d unlinked %s#%d (queue %s)", id, repo, task, removed.Queue), nil
				}
				return fmt.Sprintf("sprint #%d unlinked %s#%d", id, repo, task), nil
			})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo the run lives in")
	cmd.Flags().StringVar(&queue, "queue", "", "exact opaque queue tag when the card links this repo+id more than once")
	cmd.Flags().Int64Var(&task, "task", 0, "linked weave run/issue id in that repo")
	flags.attach(cmd)
	return cmd
}

func validateSprintRunLink(repo string, task int64) error {
	_, err := resolveAndValidateSprintRunLink(repo, task, "")
	return err
}

func resolveAndValidateSprintRunLink(repo string, task int64, queue string) (sprintRun, error) {
	queueDir, err := resolveSprintRunQueue(repo, task, queue)
	if err != nil {
		return sprintRun{}, err
	}
	q, err := loadWeaveQueue(queueDir)
	if err != nil {
		return sprintRun{}, fmt.Errorf("load weave queue for repo %q: %w", repo, err)
	}
	run := findWeaveItem(q, task)
	if run == nil {
		return sprintRun{}, fmt.Errorf("repo %q has no weave run #%d", repo, task)
	}
	cap, valid := weavePointRuntimeCap(run.Points)
	if !valid {
		if run.Points == 0 {
			return sprintRun{}, fmt.Errorf("cannot link %s#%d: run has no story points; set one of 1,2,3,5,8 first", repo, task)
		}
		return sprintRun{}, fmt.Errorf("cannot link %s#%d: invalid story points %d (want 1,2,3,5,8)", repo, task, run.Points)
	}
	// A run that has left planning may already have a wrapper or preserved
	// execution record. Linking it promises that its point estimate actually
	// bounded that execution, so fail closed on legacy/unbounded or stale
	// over-cap launch specs. A running watchdog cannot be retrofitted safely.
	if run.State != "todo" {
		if run.LaunchSpec == nil || run.LaunchSpec.MaxRuntime <= 0 {
			return sprintRun{}, fmt.Errorf("cannot link %s#%d: state %q has no bounded launch runtime; stop and restart it under its point cap first", repo, task, run.State)
		}
		if run.LaunchSpec.MaxRuntime > cap {
			return sprintRun{}, fmt.Errorf("cannot link %s#%d: launch runtime %s exceeds the %d-point cap %s; stop and restart it under its point cap first", repo, task, run.LaunchSpec.MaxRuntime, run.Points, cap)
		}
	}
	return sprintRun{
		Repo:  strings.TrimSpace(repo),
		ID:    task,
		Queue: filepath.Base(queueDir),
		Born:  run.Created,
	}, nil
}

// newWeaveCheckpointCmd is the conductor's durability heartbeat: update
// the continuity record AND refresh the lease in one call. Run it after
// each meaningful step so that if the conductor dies (SIGKILL / token
// exhaustion) the successor has a current resume brief and the lease
// goes stale on schedule.
func newWeaveCheckpointCmd() *cobra.Command {
	var flags weaveOutputFlags
	var message string
	cmd := &cobra.Command{
		Use:   "checkpoint <sprint>",
		Short: "Update a sprint's continuity record + refresh the conductor lease",
		Long: `checkpoint writes the resume brief a fresh conductor reads to continue,
and refreshes the lease heartbeat. Run it frequently — it is the
durability mechanism: a conductor that dies between checkpoints loses
only the work since the last one, and its lease goes stale so a
successor can take over.

  bashy sprint checkpoint 3 -m "P3: LWS manifest drafted (task #11 doing);
  next: wire clusterCanHold; blocker: none"`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("-m <resume brief> required")
			}
			who := weaveConductorName("")
			return runWeaveStoryMutate(cmd, id, "sprint checkpoint", &flags, func(s *weaveStory) (string, error) {
				prev, stale, free := weaveStoryLeaseState(s)
				if free {
					return "", fmt.Errorf("sprint #%d lease is unclaimed — take it explicitly before checkpointing", id)
				}
				if stale {
					return "", fmt.Errorf("sprint #%d lease is STALE (was %s) — take it explicitly to recover", id, prev)
				}
				s.Continuity = message
				s.Lease = &weaveStoryLease{Holder: prev, At: time.Now().UTC()}
				weaveStoryAppend(s, who, "progress", "checkpoint")
				return fmt.Sprintf("sprint #%d: continuity updated, lease refreshed (%s)%s",
					id, prev, sprintUnreadReminder(prev)), nil
			})
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "the resume brief")
	flags.attach(cmd)
	return cmd
}

// ---- shared helpers -------------------------------------------------

func weaveStoryAppend(s *weaveStory, author, kind, body string) {
	body = strings.TrimSpace(body)
	if s == nil || body == "" {
		return
	}
	if kind == "" {
		kind = "note"
	}
	if author == "" {
		author = "conductor"
	}
	s.Thread = append(s.Thread, weaveComment{At: time.Now().UTC(), Author: author, Kind: kind, Body: body})
	s.UpdatedAt = time.Now().UTC()
}

// weaveStoryDir resolves the USER-GLOBAL sprint board dir
// (~/.bashy/sprint) — NOT a per-repo queue. A sprint spans repos, so its
// board lives above any one repo; the board reuses the weaveQueue store
// (only its Sprints field) + the same flock, just at a repo-less dir. No
// git repo is required to manage the board.
func weaveStoryDir(cmd *cobra.Command, mode weavecli.OutputMode, op string) (string, error) {
	// THE STORE MUST BE REDIRECTABLE, and not only for tidiness.
	//
	// This resolved to $HOME/.bashy/sprint with no override of any kind, which
	// means there was no way to exercise sprint against a scratch board — and
	// the obvious guess ($BASHY_HOME, which the audit log, foreman state and
	// skills store all honour) silently did nothing. Anyone testing the verbs
	// was therefore editing the real board while believing they were not, and
	// `sprint add` plus `sprint start` are exactly the verbs someone reaches for
	// when trying a feature out. That happened during this feature's own
	// development, on live sprints.
	//
	// The ladder matches audit, foreman and the skills store, so the guess is
	// now the right one.
	dir, err := sprintStoreDir()
	if err != nil {
		return "", ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitGenericFail, err))
	}
	return dir, nil
}

// runWeaveStoryMutate is the lock→find→mutate→emit skeleton shared by
// the sprint mutators. mut returns a success line (text mode) or an error.
func runWeaveStoryMutate(cmd *cobra.Command, id int64, op string, flags *weaveOutputFlags, mut func(*weaveStory) (string, error)) error {
	mode := flags.mode()
	dir, err := weaveStoryDir(cmd, mode, op)
	if err != nil {
		return err
	}
	var line string
	lockErr := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		s := findWeaveStory(q, id)
		if s == nil {
			return fmt.Errorf("sprint #%d not found", id)
		}
		// The board is visible to the callback for the duration of the lock:
		// the closing check must know which repos another RUNNING sprint also
		// holds, and that is a property of the set, not of one card.
		prevBoard := currentBoard
		currentBoard = q.Stories
		defer func() { currentBoard = prevBoard }()
		msg, merr := mut(s)
		if merr != nil {
			return merr
		}
		line = msg
		return nil
	})
	if lockErr != nil {
		code := weavecli.ExitGenericFail
		if strings.Contains(lockErr.Error(), "not found") {
			code = weavecli.ExitInvalidArg
		}
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, code, lockErr))
	}
	if mode == weavecli.OutputJSON {
		return ec(emitOK(cmd.OutOrStdout(), mode, op, map[string]any{"sprint": id, "ok": true}))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", op, line)
	return nil
}

// sprintStoreDir resolves the sprint board's location:
//
//	$BASHY_SPRINT_DIR   the specific override, most precise
//	$BASHY_HOME/sprint  the whole bashy home relocated (tests, sandboxed runs)
//	~/.bashy/sprint     the default, unchanged
func sprintStoreDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("BASHY_SPRINT_DIR")); d != "" {
		return d, nil
	}
	if h := strings.TrimSpace(os.Getenv("BASHY_HOME")); h != "" {
		return filepath.Join(h, "sprint"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".bashy", "sprint"), nil
}

// currentBox is the running cycle, or nil when the sprint is not on the clock.
func (s *weaveStory) currentBox() *weaveStoryBox {
	if len(s.Boxes) == 0 {
		return nil
	}
	if last := &s.Boxes[len(s.Boxes)-1]; last.Running() {
		return last
	}
	return nil
}

// lastBox is the most recent cycle whether or not it still runs — what a view
// spanning PAST and PRESENT reads.
func (s *weaveStory) lastBox() *weaveStoryBox {
	if len(s.Boxes) == 0 {
		return nil
	}
	return &s.Boxes[len(s.Boxes)-1]
}

// cadence summarises what this sprint's cycles actually cost against what they
// promised. One cycle is an anecdote; the run of them is the only thing that
// says whether a cadence is real or a label.
func (s *weaveStory) cadence() (cycles int, planned, actual time.Duration) {
	for i := range s.Boxes {
		b := &s.Boxes[i]
		if b.StoppedAt == nil {
			continue // an open cycle has no final cost yet
		}
		cycles++
		planned += b.Planned
		actual += b.Elapsed(*b.StoppedAt)
	}
	return cycles, planned, actual
}

// currentBoard is the sibling stories visible during a locked mutation. It is a
// package var rather than a parameter because every existing mutate callback
// takes one story and threading a second argument through all of them would
// churn a dozen call sites to serve one. It is only ever set while the queue
// lock is held, and restored on the way out.
var currentBoard []*weaveStory
