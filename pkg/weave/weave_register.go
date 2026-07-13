// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/qiangli/coreutils/pkg/issue"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

// The bridge between the REGISTER and the QUEUE.
//
// `bashy issue` is the durable, committed record of what is wrong and what is wanted;
// `bashy weave` is the per-machine queue of work an agent is actually doing. Without a
// join they would be two systems that disagree — a closed issue still "in progress" on
// someone's laptop, a merged branch whose requirement is still open. So there is
// exactly one way for a register entry to become work, and it links both directions:
// the queue item remembers which issue it implements (weaveItem.Register), and the
// issue remembers which queue item is in flight (Issue.Weave).
//
// A TRIAGED issue only. An untriaged issue is a thought nobody has accepted or scoped —
// handing it straight to an agent is how a fleet spends an afternoon building something
// that was never wanted. `issue triage` is the gate, and it forces a stage.

func runWeaveAddFromIssue(cmd *cobra.Command, ref string, flags *weaveOutputFlags) error {
	mode := flags.mode()
	cwd, _ := os.Getwd()
	root, err := weaveRepoRoot(cwd)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitPrecondFail, err))
	}

	reg := issue.New(root)
	it, err := reg.Resolve(ref)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitInvalidArg, err))
	}
	if it.Status == issue.StatusClosed {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitInvalidArg,
			fmt.Errorf("issue %s is closed (%s) — `bashy issue reopen %s` first", it.ID[:8], it.Resolution, it.ID[:8])))
	}
	if it.Status != issue.StatusTriaged {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitPrecondFail,
			fmt.Errorf("issue %s is %s, not triaged\n\nAn untriaged issue is a thought nobody has accepted or scoped. Handing it\nstraight to an agent is how a fleet spends an afternoon building something\nnobody wanted.\n\n  bashy issue triage %s --stage <plan|code|test|deploy>",
				it.ID[:8], it.Status, it.ID[:8])))
	}
	if it.Weave != 0 {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitPrecondFail,
			fmt.Errorf("issue %s is already in flight as weave #%d (`weave status %d`)", it.ID[:8], it.Weave, it.Weave)))
	}

	dir, err := weaveQueueDir(root)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitGenericFail, err))
	}
	prio := it.Priority
	if prio == "" {
		prio = "p2"
	}
	body := it.Body
	if len(it.Refs) > 0 {
		// The agent must be told the issue is not confined to this repo, or it will
		// "fix" the symptom here rather than the cause next door.
		body += fmt.Sprintf("\n\nThis issue also touches: %v\n", it.Refs)
	}

	var qi *weaveItem
	lockErr := withWeaveQueueLock(dir, func(q *weaveQueue) error {
		qi = &weaveItem{
			ID:       q.NextID,
			Title:    it.Title,
			Body:     body,
			Priority: prio,
			Stage:    it.Stage,
			Register: it.ID,
			State:    "todo",
			Created:  timeNowUTC(),
		}
		q.NextID++
		q.Items = append(q.Items, qi)
		return nil
	})
	if lockErr != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitGenericFail, lockErr))
	}

	// Link back, so `issue show` knows the work is in flight and a second agent
	// cannot queue the same issue twice.
	it.Weave = qi.ID
	if _, err := reg.Save(it); err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitGenericFail, err))
	}

	if mode == weavecli.OutputJSON {
		return ec(emitOK(cmd.OutOrStdout(), mode, "weave add", map[string]any{
			"issue":    qi.ID,
			"register": it.ID,
			"title":    qi.Title,
			"stage":    itemStage(qi),
			"priority": qi.Priority,
			"state":    qi.State,
		}))
	}
	fmt.Fprintf(cmd.OutOrStdout(), "weave add: run #%d created from register %s (%s/%s, todo) — %q\n",
		qi.ID, it.ID[:8], qi.Priority, itemStage(qi), qi.Title)
	return nil
}

// weaveCloseRegisterOnMerge settles the register entry when its work lands.
//
// Called when an item reaches "done". Without it the register would drift out of date
// the moment work succeeded — and a stale register is worse than none, because people
// trust it.
func weaveCloseRegisterOnMerge(root, base string, it *weaveItem) {
	if it.Register == "" {
		return
	}
	if it.CommitsAhead <= 0 || !weaveItemMerged(root, base, it) {
		return
	}
	reg := issue.New(root)
	ri, err := reg.Resolve(it.Register)
	if err != nil || ri.Status == issue.StatusClosed {
		return
	}
	now := timeNowUTC()
	ri.Status = issue.StatusClosed
	ri.Closed = &now
	ri.Resolution = "fixed"
	ri.ClosedBy = it.Owner
	ri.Weave = 0
	_, _ = reg.Save(ri)
}
