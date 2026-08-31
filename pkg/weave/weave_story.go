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
type weaveStoryLease struct {
	Holder string    `json:"holder"`
	At     time.Time `json:"at"`
}

const sprintLeaseTTL = 30 * time.Minute

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
// holder + fresh/STALE/free. Stale = heartbeat older than sprintLeaseTTL.
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
// The stored shape is unchanged: a sprint lease records only a holder and a
// timestamp, and the TTL is this package's constant. What changes is the
// VERDICT — a lease with no timestamp now reads as unknown rather than as
// silently fresh, which is what a zero time compared against a TTL used to
// produce.
func (s *weaveStory) seat() role.Seat {
	if s == nil || s.Lease == nil {
		return role.Seat{TTL: sprintLeaseTTL}
	}
	return role.Seat{
		Holder: s.Lease.Holder,
		// A sprint lease has one timestamp doing both jobs: it is stamped on
		// take and refreshed on checkpoint, so it is the heartbeat. Reporting
		// it as the acquisition time too would make every refresh look like a
		// new tenure.
		HeartbeatAt: s.Lease.At,
		TTL:         sprintLeaseTTL,
	}
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

// weaveStoryConductorName bridges ephemeral CLI identity with the durable
// sprint lease. `sprint take --as X` and a later `sprint start` are separate
// processes; when the latter has no identity evidence, the live holder is the
// only truthful actor. Explicit process identity still wins so a different
// agent cannot silently act through somebody else's lease.
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
			fmt.Fprintf(out, "  #%d %s%s%s%s\n", s.ID, epicTag, weaveTruncate(s.Title, 52), lease, box)
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
resume → end; handoff/take/stop remain the lower-level compatibility verbs.`,
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
		newSprintStatusCmd(),
		newSprintWhoCmd(),
		newSprintPingCmd(),
		newSprintStartCmd(),
		newSprintPauseCmd(),
		newSprintResumeCmd(),
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
				}
				from := s.Column
				s.Column = col
				weaveStoryAppend(s, weaveStoryConductorName(s, ""), "system", fmt.Sprintf("moved %s → %s", from, col))
				return fmt.Sprintf("sprint #%d %s → %s", id, from, col), nil
			})
		},
	}
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryTakeCmd() *cobra.Command {
	var flags weaveOutputFlags
	var as string
	var force bool
	cmd := &cobra.Command{
		Use:   "take <sprint>",
		Short: "Claim the conductor lease on a sprint (takes over a STALE/dead conductor)",
		Long: `take claims the conductor lease so a new conductor can pick up a sprint —
the human-directed switch, or recovery after the previous conductor died
(SIGKILL / token exhaustion). A free or STALE lease is taken directly; a
FRESH lease requires --force (and is recorded). After taking, read the
continuity record (sprint show) and resume.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			return runWeaveStoryMutate(cmd, id, "sprint take", &flags, func(s *weaveStory) (string, error) {
				who := sprintTakeoverIdentity(s, as)
				prev, stale, free := weaveStoryLeaseState(s)
				if !free && !stale && prev != who && !force {
					return "", fmt.Errorf("sprint #%d lease is held by %s (fresh) — coordinate, or --force to take over", id, prev)
				}
				// THE DEAD HOLDER CANNOT CLOSE THEIR OWN ROOM. A successor
				// taking over a stale lease inherits a channel addressed to
				// somebody who will never read it, so the takeover closes it
				// and opens one that answers.
				if s.Contact != nil {
					_ = closeSprintRoom(s, weaveStoryConductorName(s, ""))
				}
				if s.currentBox().Running() {
					if c, err := openSprintRoom(s, who); err == nil {
						s.Contact = c
					}
				}
				s.Lease = &weaveStoryLease{Holder: who, At: time.Now().UTC()}
				s.Owner = who
				switch {
				case free:
					weaveStoryAppend(s, who, "system", "took conductor lease (was unclaimed)")
				case stale:
					weaveStoryAppend(s, who, "system", fmt.Sprintf("took STALE conductor lease from %s (recovery)", prev))
				default:
					weaveStoryAppend(s, who, "system", fmt.Sprintf("force-took conductor lease from %s", prev))
				}
				return fmt.Sprintf("sprint #%d: %s is now conductor — use this exact name for mb/Meet/chat/ping; %s; read sprint show %d", id, who, sprintReadyLine(who), id), nil
			})
		},
	}
	cmd.Flags().StringVar(&as, "as", "", "conductor name (default $WEAVE_CONDUCTOR/$WEAVE_AGENT)")
	cmd.Flags().BoolVar(&force, "force", false, "take over a fresh lease")
	flags.attach(cmd)
	return cmd
}

func newWeaveStoryHandoffCmd() *cobra.Command {
	var flags weaveOutputFlags
	var message string
	cmd := &cobra.Command{
		Use:   "handoff <sprint>",
		Short: "Graceful handoff: checkpoint continuity + release the conductor lease",
		Long: `handoff is the graceful conductor exit (e.g. on Ctrl+C, planned switch,
or running low on context): record a final continuity brief and RELEASE
the lease so the next conductor takes over cleanly. In-flight tasks are
untouched — they survive in the queue for the successor.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			return runWeaveStoryMutate(cmd, id, "sprint handoff", &flags, func(s *weaveStory) (string, error) {
				// Releasing the role closes its room. A closed room with work
				// still running would be a dead letterbox, but a handoff is
				// precisely the case where the work moves WITH the role — the
				// successor opens their own.
				who := weaveStoryConductorName(s, "")
				_ = closeSprintRoom(s, who)
				if strings.TrimSpace(message) != "" {
					s.Continuity = message
				}
				weaveStoryAppend(s, who, "system", "handed off — released conductor lease")
				s.Lease = nil
				return fmt.Sprintf("sprint #%d: lease released; continuity recorded for the next conductor", id), nil
			})
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "final continuity brief (resume instructions for the successor)")
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

The conductor PROCESS itself is not killed (the lease is heartbeat-based,
not pid-tracked) — but with no lease and no runs it can do nothing; kill it
at the OS level if it is still polling.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			who := weaveConductorName("")
			return runWeaveStoryMutate(cmd, id, "sprint abort", &flags, func(s *weaveStory) (string, error) {
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
				return fmt.Sprintf("sprint #%d: continuity updated, lease refreshed (%s)", id, prev), nil
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
