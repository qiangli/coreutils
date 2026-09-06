---
id: 66a612deb300
kind: task
title: weave prune cannot see workspace dirs with no queue item
seq: 68
status: done
priority: p1
created: 2026-09-06T10:43:43.115005Z
sprint: 115
closed: 2026-09-06T14:06:54.131028Z
---

INVARIANT (weave's own contract, from `weave prune --help`): "Removes lingering
workspace directories for done, abandoned, failed, and killed items." When every
run is terminal, <queueDir>/workspaces/ should be EMPTY.

DEFECT: runWeavePrune (pkg/weave/weave_impl.go) sweeps by iterating the QUEUE:

    for _, it := range q.Items {
        if weavePrunableForSweep(it.State, stale) { pendingCount++ }
    }

It never reads the workspaces/ directory. Any subdirectory there without a
backing queue item is therefore invisible to prune FOREVER — no flag reaches it,
--stale included, because --stale also filters queue items.

MEASURED: two stores held sibling-dep mirror clones with no queue item at all
(coreutils, filebrowser, readline, sh — 266 MB). No weave command could reclaim
them; they had to be removed by hand.

This is the same LEAK CLASS that weaveWorkspaceOwner already documents having
produced 237 empty directories. That fix stopped new ROOTS from forking; orphan
WORKSPACES under an existing root are still unswept.

FIX: enumerate <queueDir>/workspaces/* and treat an entry with no owning queue
item as prunable, under the SAME work guard used for items — refuse when the
tree is dirty or an agent branch holds unmerged commits, report the reason, and
require --force to override. Never remove a directory holding work.
