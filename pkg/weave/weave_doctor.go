package weave

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/weavecli"
)

// `weave doctor` — run the reaper on demand and report the lifecycle audit.
//
// The reaper also runs on `weave list` and each heartbeat tick, so doctor is
// rarely the thing that FIXES a queue. Its job is to make the invariant
// inspectable: what did the machine just close, and for everything still open,
// what is the named next step. A queue in which some item's answer is "nothing"
// is a queue with a limbo in it, and doctor is where that shows up.

func newWeaveDoctorCmd() *cobra.Command {
	var flags weaveOutputFlags
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Reap limbo states and audit the lifecycle of every open item",
		Long: `doctor runs the lifecycle REAPER over this repo's queue and then audits
what is left.

The reaper gives every stuck state a determinate exit:

  allocated whose launcher died      -> failed
  finalizing whose conductor died    -> working (wrapper alive) or failed
  working whose wrapper pid is dead  -> failed (wrapper-died)
  submitted already merged into base -> done
  submitted past the threshold       -> flagged needs-steward
  failed/killed sitting on commits   -> flagged salvageable

It NEVER destroys committed work: it writes state fields and flags only.
Removing a workspace stays an explicit guarded step (` + "`weave prune`" + `,
` + "`weave abandon`" + `). It never promotes a crash to success either — a dead
wrapper becomes failed, and any work behind it is surfaced, not merged.

The audit then lists every item that is not closed (done/abandoned) with the
next step that will close it. Every state has one; see
docs/weave-lifecycle-state-machine.md for the whole machine.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWeaveDoctor(cmd, &flags)
		},
	}
	flags.attach(cmd)
	return cmd
}

// weaveOpenItem is one not-yet-closed item plus the named step that closes it.
type weaveOpenItem struct {
	Issue     int64  `json:"issue"`
	State     string `json:"state"`
	Title     string `json:"title,omitempty"`
	NextSteps string `json:"next_steps"`
	Flag      string `json:"flag,omitempty"`
}

func runWeaveDoctor(cmd *cobra.Command, flags *weaveOutputFlags) error {
	mode := flags.mode()
	const op = "weave doctor"

	cwd, _ := os.Getwd()
	root, err := weaveRepoRoot(cwd)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitPrecondFail, err))
	}
	dir, err := weaveQueueDir(root)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitGenericFail, err))
	}
	base := weaveBaseBranch(root)
	actions, err := weaveReapQueue(dir, root, base)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitGenericFail, err))
	}
	q, err := loadWeaveQueue(dir)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op, weavecli.ExitGenericFail, err))
	}

	open := make([]weaveOpenItem, 0, len(q.Items))
	for _, it := range weaveLimboItems(q) {
		row := weaveOpenItem{Issue: it.ID, State: it.State, Title: it.Title, NextSteps: weaveNextSteps(it)}
		switch {
		case it.NeedsSteward:
			row.Flag = "needs-steward"
		case it.Salvageable:
			row.Flag = "salvageable"
		}
		open = append(open, row)
	}

	if mode == weavecli.OutputJSON {
		reaped := make([]map[string]any, 0, len(actions))
		for _, a := range actions {
			reaped = append(reaped, map[string]any{
				"issue": a.Issue, "from": a.From, "to": a.To, "reason": a.Reason, "flag": a.Flag,
			})
		}
		return ec(emitOK(cmd.OutOrStdout(), mode, op, map[string]any{
			"reaped": reaped,
			"open":   open,
		}))
	}

	w := cmd.OutOrStdout()
	if len(actions) == 0 {
		fmt.Fprintln(w, "reaped: nothing (no item was in a stuck state)")
	} else {
		fmt.Fprintf(w, "reaped (%d):\n", len(actions))
		for _, a := range actions {
			fmt.Fprintf(w, "  %s\n", a)
		}
	}
	if len(open) == 0 {
		fmt.Fprintln(w, "open: none — every item is closed (done/abandoned)")
		return nil
	}
	fmt.Fprintf(w, "open (%d) — each with the step that closes it:\n", len(open))
	for _, o := range open {
		flag := ""
		if o.Flag != "" {
			flag = " [" + o.Flag + "]"
		}
		fmt.Fprintf(w, "  #%-4d %-11s%s %s\n", o.Issue, o.State, flag, o.NextSteps)
	}
	return nil
}

// weaveNextSteps names what will close an item from its current state. It is
// derived from the declared transition table, not hand-maintained prose, so a
// state added without a way out cannot quietly acquire a plausible-sounding
// answer here — it gets the empty set, and TestLifecycleHasNoLimbo fails.
func weaveNextSteps(it *weaveItem) string {
	if it.NeedsSteward && it.StewardReason != "" {
		return it.StewardReason
	}
	if it.Salvageable {
		return fmt.Sprintf("committed work survives this %s run — `weave salvage %d` then `weave pull`, or `weave abandon %d`", it.State, it.ID, it.ID)
	}
	var steps []string
	for _, t := range weaveLifecycleTransitionsFrom(it.State) {
		steps = append(steps, fmt.Sprintf("%s -> %s (%s)", t.From, t.To, t.By))
	}
	if len(steps) == 0 {
		// Unreachable while the lifecycle test passes; loud rather than blank
		// if a state ever escapes the table.
		return "NO DECLARED TRANSITION — this is a limbo; see docs/weave-lifecycle-state-machine.md"
	}
	return steps[0]
}
