package weave

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// Baton is the CONDUCTOR handoff note for a local weave campaign: the intent,
// strategy, and next moves that the queue itself does NOT capture, so a fresh
// conductor can pick up from `weave baton` + a live `weave list` without reading
// code or docs. It is rewritten at each handoff stage (mid-sprint on trouble,
// end-of-sprint). Stored at <queueDir>/baton.json — per-campaign, system-wide.
type Baton struct {
	Goal        string    `json:"goal"`         // the campaign's north star + done-criteria
	Stage       string    `json:"stage"`        // current sprint/phase, e.g. "sprint 2 of 4"
	Plan        string    `json:"plan"`         // sprint plan / decomposition strategy
	Done        []string  `json:"done"`         // what's merged/verified (one line each)
	NextActions []string  `json:"next_actions"` // the next conductor's first moves
	Lessons     []string  `json:"lessons"`      // gotchas / routing decisions / tool notes
	Notes       string    `json:"notes"`        // free narrative
	WrittenBy   string    `json:"written_by"`
	WrittenAt   time.Time `json:"written_at"`
}

func batonPath(queueDir string) string { return filepath.Join(queueDir, "baton.json") }

func loadBaton(queueDir string) (*Baton, bool) {
	b, err := os.ReadFile(batonPath(queueDir))
	if err != nil {
		return nil, false
	}
	var bt Baton
	if json.Unmarshal(b, &bt) != nil {
		return nil, false
	}
	return &bt, true
}

func saveBaton(queueDir string, bt *Baton) error {
	bt.WrittenAt = time.Now()
	b, err := json.MarshalIndent(bt, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(batonPath(queueDir), b, 0o644)
}

// renderBaton formats the baton as a self-contained markdown handoff brief — the
// thing a fresh conductor reads first. It ends with the exact live-state
// commands so the new conductor reconciles intent (this note) with current
// reality (the queue), then resumes.
func renderBaton(bt *Baton) string {
	var s strings.Builder
	fmt.Fprintf(&s, "# Conductor baton")
	if bt.Stage != "" {
		fmt.Fprintf(&s, " — %s", bt.Stage)
	}
	s.WriteString("\n\n")
	if bt.Goal != "" {
		fmt.Fprintf(&s, "## Goal\n%s\n\n", bt.Goal)
	}
	if bt.Plan != "" {
		fmt.Fprintf(&s, "## Plan\n%s\n\n", bt.Plan)
	}
	writeList := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&s, "## %s\n", title)
		for _, it := range items {
			fmt.Fprintf(&s, "- %s\n", it)
		}
		s.WriteString("\n")
	}
	writeList("Done (merged/verified)", bt.Done)
	writeList("Next actions", bt.NextActions)
	writeList("Lessons / routing", bt.Lessons)
	if bt.Notes != "" {
		fmt.Fprintf(&s, "## Notes\n%s\n\n", bt.Notes)
	}
	by := bt.WrittenBy
	if by == "" {
		by = "unknown"
	}
	fmt.Fprintf(&s, "_baton written by %s at %s_\n\n", by, bt.WrittenAt.Local().Format(time.RFC3339))
	s.WriteString("## Reconcile with live state before resuming\n")
	s.WriteString("- `bashy weave list`               — issue states (todo/working/submitted/merged/failed)\n")
	s.WriteString("- `bashy weave fleet --probe`       — which tools are available right now\n")
	s.WriteString("- `bashy weave fleet interview --all` — per-tool launch contracts + role ratings\n")
	s.WriteString("- `bashy weave guide`              — the conductor playbook\n")
	return s.String()
}

func newWeaveBatonCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{
		Use:   "baton",
		Short: "Show the conductor handoff note (intent + next moves) for picking up a campaign",
		Long: `baton is the CONDUCTOR handoff note for THIS campaign. The current
conductor writes it ('weave baton write …') at each handoff stage — mid-sprint
when it must drop (ratelimit / token overuse / failure) or at end-of-sprint — so
the next conductor resumes from the intent + strategy + next moves WITHOUT
reading code or docs, then reconciles against live 'weave list'. (For the
cloudbox shared-session lease handoff, see 'weave handoff'.)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveBatonShow(cmd, &flags)
		},
	}
	flags.attach(cmd)
	cmd.AddCommand(newWeaveBatonWriteCmd())
	return cmd
}

func runWeaveBatonShow(cmd *cobra.Command, flags *weaveOutputFlags) error {
	mode := flags.mode()
	cwd, _ := os.Getwd()
	root, err := weaveRepoRoot(cwd)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave baton", weavecli.ExitPrecondFail, err))
	}
	dir, err := weaveQueueDir(root)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave baton", weavecli.ExitGenericFail, err))
	}
	bt, ok := loadBaton(dir)
	if !ok {
		if mode == weavecli.OutputJSON {
			return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave baton", map[string]any{"baton": nil}))
		}
		fmt.Fprintln(cmd.OutOrStdout(), "no baton yet — the conductor writes one with `weave baton write …`")
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave baton", nil))
	}
	if mode == weavecli.OutputJSON {
		return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave baton", map[string]any{"baton": bt}))
	}
	fmt.Fprint(cmd.OutOrStdout(), renderBaton(bt))
	return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave baton", nil))
}

func newWeaveBatonWriteCmd() *cobra.Command {
	var flags weaveOutputFlags
	var bt Baton
	var next, lessons, done []string
	cmd := &cobra.Command{
		Use:   "write",
		Short: "Write/update the conductor handoff note for this campaign",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := flags.mode()
			cwd, _ := os.Getwd()
			root, err := weaveRepoRoot(cwd)
			if err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave baton write", weavecli.ExitPrecondFail, err))
			}
			dir, err := weaveQueueDir(root)
			if err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave baton write", weavecli.ExitGenericFail, err))
			}
			// Merge onto any existing baton so partial updates accrue.
			cur, _ := loadBaton(dir)
			if cur == nil {
				cur = &Baton{}
			}
			if bt.Goal != "" {
				cur.Goal = bt.Goal
			}
			if bt.Stage != "" {
				cur.Stage = bt.Stage
			}
			if bt.Plan != "" {
				cur.Plan = bt.Plan
			}
			if bt.Notes != "" {
				cur.Notes = bt.Notes
			}
			if bt.WrittenBy != "" {
				cur.WrittenBy = bt.WrittenBy
			}
			if len(done) > 0 {
				cur.Done = append(cur.Done, done...)
			}
			if len(next) > 0 {
				cur.NextActions = next // next actions REPLACE (they're the current to-do)
			}
			if len(lessons) > 0 {
				cur.Lessons = append(cur.Lessons, lessons...)
			}
			if err := saveBaton(dir, cur); err != nil {
				return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave baton write", weavecli.ExitGenericFail, err))
			}
			if mode != weavecli.OutputJSON {
				fmt.Fprintln(cmd.OutOrStdout(), "baton written — next conductor: `bashy weave baton`")
			}
			return ec(weavecli.EmitOK(cmd.OutOrStdout(), mode, "weave baton write", map[string]any{"written_by": cur.WrittenBy}))
		},
	}
	flags.attach(cmd)
	cmd.Flags().StringVar(&bt.Goal, "goal", "", "Campaign north star + done-criteria")
	cmd.Flags().StringVar(&bt.Stage, "stage", "", "Current stage, e.g. 'sprint 2 of 4'")
	cmd.Flags().StringVar(&bt.Plan, "plan", "", "Sprint plan / decomposition strategy")
	cmd.Flags().StringVar(&bt.Notes, "notes", "", "Free narrative")
	cmd.Flags().StringVar(&bt.WrittenBy, "by", "", "Who is handing off (tool/agent name)")
	cmd.Flags().StringArrayVar(&done, "done", nil, "Append a merged/verified item (repeatable)")
	cmd.Flags().StringArrayVar(&next, "next", nil, "Next action for the new conductor (repeatable; replaces prior)")
	cmd.Flags().StringArrayVar(&lessons, "lesson", nil, "Append a lesson/routing note (repeatable)")
	return cmd
}
