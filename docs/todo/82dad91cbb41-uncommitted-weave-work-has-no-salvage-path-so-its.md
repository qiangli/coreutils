---
id: 82dad91cbb41
kind: task
title: uncommitted weave work has no salvage path, so its workspace is unfreeable
seq: 69
status: done
priority: p1
created: 2026-09-06T10:43:55.580223Z
sprint: 115
closed: 2026-09-06T14:06:54.199866Z
---

DEFECT: committed and uncommitted work are treated asymmetrically, so a terminal
run holding only an uncommitted tree strands its workspace permanently.

weavePruneHoldReason (pkg/weave/weave_impl.go) shows it directly:

    if ahead > 0             -> "... `weave salvage <id>` to keep them"   // verb
    if dirty+untracked > 0   -> "%d uncommitted file(s)"                  // none

Committed work can be promoted OUT of the workspace: `weave abandon --force`
writes refs/salvage/abandoned-<id> into the user's repo BEFORE destroying
anything, after which the directory is disposable. Uncommitted work has no such
path, so the only choices are keep-forever or destroy-with---force.

MEASURED: runs #54/#60 (committed) were freed and preserved as salvage refs.
Runs #45/#51 (uncommitted) were refused identically by prune AND by
`prune --force` — --force covers unmerged commits but NOT a dirty tree. Freeing
#51 required a manual `git checkout -- .`, which is exactly the destructive act
the guard exists to prevent.

ROOT CAUSE: the promotion that would have prevented this is gated off. At
weave_impl.go the terminal path computes

    autoCommitEligible := opts.autoCommit && (verify ok) && (dirty || untracked)

and its own comment says committing "costs nothing and risks nothing" because the
work lands on an ISOLATED branch that can only reach base through the gate, and
weaveTerminalState still refuses `submitted` on a non-zero exit. Yet it depends
on the per-run opts.autoCommit, so a killed run without it leaves the tree loose.

FIX: give uncommitted work the same exit as committed work. On the terminal path
commit a dirty tree onto the agent branch by default (state unchanged), and make
--force preserve before destroying in BOTH branches — commit the tree, write the
salvage ref, then remove. weavePruneHoldReason must then offer a recovery verb in
both branches instead of only one.
