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
	Epic       string           `json:"epic,omitempty"`       // grouping label
	SpecRef    string           `json:"spec_ref,omitempty"`   // handoff/spec doc reference
	Acceptance string           `json:"acceptance,omitempty"` // done criteria
	Column     string           `json:"column"`               // backlog|doing|review|done
	Continuity string           `json:"continuity,omitempty"` // the resume brief
	Lease      *weaveStoryLease `json:"lease,omitempty"`      // current conductor + heartbeat
	Thread     []weaveComment   `json:"thread,omitempty"`     // sprint-level history
	Runs       []sprintRun      `json:"runs,omitempty"`       // linked weave runs, CROSS-REPO
	Created    time.Time        `json:"created"`
	UpdatedAt  time.Time        `json:"updated_at,omitempty"`
}

// sprintRun links a sprint to a weave run (issue) in a SPECIFIC repo.
// A sprint spans repos (sh/bashy/outpost/…); each repo keeps its own
// per-repo weave queue, so a link is (repo, id) — e.g. {outpost, 11}.
type sprintRun struct {
	Repo string `json:"repo"`
	ID   int64  `json:"id"`
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
	if s.Lease == nil || s.Lease.Holder == "" {
		return "", false, true
	}
	return s.Lease.Holder, time.Since(s.Lease.At) > sprintLeaseTTL, false
}

// weaveConductorName resolves the acting conductor's name: --as flag >
// $WEAVE_CONDUCTOR > $WEAVE_AGENT > "conductor".
func weaveConductorName(asFlag string) string {
	for _, v := range []string{asFlag, os.Getenv("WEAVE_CONDUCTOR"), os.Getenv("WEAVE_AGENT")} {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
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
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "sprint board", map[string]any{"stories": stories}))
	}
	out := cmd.OutOrStdout()
	if len(stories) == 0 {
		fmt.Fprintln(out, "sprint board is empty — `sprint add \"<title>\" --spec <doc>`")
		return nil
	}
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
			fmt.Fprintf(out, "  #%d %s%s%s\n", s.ID, epicTag, weaveTruncate(s.Title, 56), lease)
		}
		if !any {
			fmt.Fprintln(out, "  —")
		}
	}
	return nil
}

// NewSprintCmd is the PLAN/HANDOFF surface — peer to `bashy weave`. A
// SPRINT spans repos/teams (e.g. "ollama feature" across sh/bashy/
// outpost; like an agile sprint across frontend/backend/cicd/qa); its
// board is USER-GLOBAL. `bashy weave` is the per-repo EXECUTION engine
// for the runs a sprint links. `bashy sprint` with no subcommand shows
// the board.
func NewSprintCmd() *cobra.Command {
	var flags weaveOutputFlags
	var epic string
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "Plan/handoff: the cross-repo sprint kanban above weave's per-repo runs",
		Long: `sprint is the conductor's PLAN/HANDOFF layer — the cross-repo/cross-team
kanban above weave. A sprint (e.g. "ollama feature") spans repos
(sh/bashy/outpost/…); its runs are executed per-repo by ` + "`bashy weave`" + `.
The board is user-global; ` + "`bashy sprint`" + ` (no subcommand) shows it.

Each sprint card carries a spec-ref, acceptance, a kanban column, a
CONTINUITY record (the resume brief), a conductor LEASE, and cross-repo
run links {repo, id}. Durability (survive Ctrl+C / SIGKILL / token
exhaustion): checkpoint often, handoff on a clean exit, take to pick up.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveBoard(cmd, epic, &flags) // default = board
		},
	}
	cmd.Flags().StringVar(&epic, "epic", "", "filter to one epic")
	flags.attach(cmd)
	cmd.AddCommand(
		newWeaveBoardCmd(),
		newWeaveStoryAddCmd(),
		newWeaveStoryShowCmd(),
		newWeaveStoryMoveCmd(),
		newWeaveStoryTakeCmd(),
		newWeaveStoryHandoffCmd(),
		newWeaveStoryCommentCmd(),
		newWeaveStoryLinkCmd(),
		newWeaveCheckpointCmd(),
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
			Created: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		weaveStoryAppend(s, "conductor", "system", fmt.Sprintf("created in %s", column))
		q.Stories = append(q.Stories, s)
		return nil
	})
	if lockErr != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "sprint add", weavecli.ExitGenericFail, lockErr))
	}
	if mode == weavecli.OutputJSON {
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "sprint add", map[string]any{"sprint": newID, "column": column}))
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
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "sprint show", map[string]any{"sprint": s}))
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
	if h, stale, free := weaveStoryLeaseState(s); !free {
		st := "fresh"
		if stale {
			st = fmt.Sprintf("STALE (no heartbeat for %s — take it)", time.Since(s.Lease.At).Round(time.Minute))
		}
		fmt.Fprintf(out, "  conductor:  %s (%s)\n", h, st)
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
				from := s.Column
				s.Column = col
				weaveStoryAppend(s, weaveConductorName(""), "system", fmt.Sprintf("moved %s → %s", from, col))
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
			who := weaveConductorName(as)
			return runWeaveStoryMutate(cmd, id, "sprint take", &flags, func(s *weaveStory) (string, error) {
				prev, stale, free := weaveStoryLeaseState(s)
				if !free && !stale && prev != who && !force {
					return "", fmt.Errorf("sprint #%d lease is held by %s (fresh) — coordinate, or --force to take over", id, prev)
				}
				s.Lease = &weaveStoryLease{Holder: who, At: time.Now().UTC()}
				switch {
				case free:
					weaveStoryAppend(s, who, "system", "took conductor lease (was unclaimed)")
				case stale:
					weaveStoryAppend(s, who, "system", fmt.Sprintf("took STALE conductor lease from %s (recovery)", prev))
				default:
					weaveStoryAppend(s, who, "system", fmt.Sprintf("force-took conductor lease from %s", prev))
				}
				return fmt.Sprintf("sprint #%d: %s is now conductor — read the continuity record (sprint show %d)", id, who, id), nil
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
				who := weaveConductorName("")
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
	var repo string
	cmd := &cobra.Command{
		Use:   "link <sprint> --repo <name> --task <issue>",
		Short: "Link a weave run (repo + issue) to a sprint — runs are cross-repo",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("sprint must be an integer: %q", args[0])
			}
			if task <= 0 || strings.TrimSpace(repo) == "" {
				return fmt.Errorf("--repo <name> and --task <issue> required (a run lives in a specific repo)")
			}
			return runWeaveStoryMutate(cmd, id, "sprint link", &flags, func(s *weaveStory) (string, error) {
				for _, r := range s.Runs {
					if r.Repo == repo && r.ID == task {
						return fmt.Sprintf("sprint #%d already links %s#%d", id, repo, task), nil
					}
				}
				s.Runs = append(s.Runs, sprintRun{Repo: repo, ID: task})
				return fmt.Sprintf("sprint #%d linked %s#%d", id, repo, task), nil
			})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repo the run lives in (e.g. outpost, bashy, sh)")
	cmd.Flags().Int64Var(&task, "task", 0, "weave run/issue id in that repo")
	flags.attach(cmd)
	return cmd
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
				s.Continuity = message
				s.Lease = &weaveStoryLease{Holder: who, At: time.Now().UTC()}
				weaveStoryAppend(s, who, "progress", "checkpoint")
				return fmt.Sprintf("sprint #%d: continuity updated, lease refreshed (%s)", id, who), nil
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
	home, err := os.UserHomeDir()
	if err != nil {
		return "", ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitGenericFail, err))
	}
	dir := filepath.Join(home, ".bashy", "sprint")
	if err := os.MkdirAll(dir, 0o755); err != nil {
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
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, op, map[string]any{"sprint": id, "ok": true}))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", op, line)
	return nil
}
