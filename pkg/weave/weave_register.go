// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package weave

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	todopkg "github.com/qiangli/coreutils/pkg/todo"
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

	// The per-repo todo list (docs/todo/) is the register: a listed todo IS an accepted,
	// wanted task, so there is no separate triage gate — being on the repo's committed
	// list is the acceptance. A done item is the only thing that must not seed a run.
	reg := todopkg.RepoStore(root)
	it, err := reg.Resolve(ref)
	if err != nil {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitInvalidArg, err))
	}
	if it.Status == todopkg.StatusDone {
		return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, "weave add", weavecli.ExitInvalidArg,
			fmt.Errorf("todo %s is already done — `bashy todo --repo status %s doing` to reopen it first", it.ID[:8], it.ID[:8])))
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
	// cannot queue the same issue twice. Auto-status: the todo is now ASSIGNED to an
	// agent (the delegation happened) — the list reflects live work without a manual
	// `todo status`. weaveCloseRegisterOnMerge flips it to done; a failed/abandoned run
	// clears the link and reverts it to todo (weaveReleaseRegister).
	it.Weave = qi.ID
	if it.Status == todopkg.StatusTodo || it.Status == todopkg.StatusBlocked {
		it.Status = todopkg.StatusAssigned
	}
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
	reg := todopkg.RepoStore(root)
	ri, err := reg.Resolve(it.Register)
	if err != nil || ri.Status == todopkg.StatusDone {
		return
	}
	now := timeNowUTC()
	ri.Status = todopkg.StatusDone
	ri.Closed = &now
	ri.Resolution = "fixed"
	ri.ClosedBy = it.Owner
	ri.Weave = 0
	_, _ = reg.Save(ri)
}

// weaveReleaseRegister reverts a linked todo when its run is ABANDONED — the user
// explicitly drops the work. A retryable failure/kill is NOT an abandonment (the item
// stays "assigned" to be re-run); only dropping it returns the todo to the backlog.
// Clears the link and reverts assigned -> todo, so the list never shows a stale
// "assigned" for work nobody will do.
func weaveReleaseRegister(root string, it *weaveItem) {
	if it == nil || it.Register == "" {
		return
	}
	reg := todopkg.RepoStore(root)
	ri, err := reg.Resolve(it.Register)
	if err != nil {
		return
	}
	changed := false
	if ri.Weave == it.ID {
		ri.Weave = 0
		changed = true
	}
	if ri.Status == todopkg.StatusAssigned {
		ri.Status = todopkg.StatusTodo
		changed = true
	}
	if changed {
		_, _ = reg.Save(ri)
	}
}
