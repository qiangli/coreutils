---
id: 87a8d553f9c0
kind: task
title: No dry-run for sprint stop and end, so preconditions can only be probed by attempting the close
seq: 78
status: todo
priority: p1
created: 2026-09-06T11:30:33.905216Z
sprint: 130
---

There is no way to ask sprint stop or sprint end whether a close would be
accepted. The checks - workers parked, repos committed/pushed/pinned, gate
green - are only reachable by attempting the close, and the gate is the LAST
of them.

That shape has a trap in it, and it was sprung on 2026-09-06. Wanting to know
whether sprint 101 would refuse on its dirty tree, the operator-facing move
was to attempt the close with a placeholder gate. sprint end 101 --gate true
did not refuse: it ran the no-op, recorded "gate green" on the card, and ended
the sprint. end has no --force and no undo, so the vacuous verdict is permanent
and reads exactly like a real one to the next reader. It had to be corrected
with a review comment on the thread naming the gate as a no-op.

That is a defence-in-depth failure of the fleet evidence invariant: a success
state reached through the ABSENCE of evidence. The proximate error was the
operator's. The missing verb is what made it the obvious move.

DO: add --dry-run to stop and end - report every closing condition and stop
before running the gate or mutating the card. sprint prune already inspects
repo state and sprint tick already gathers a worksheet without writing, so the
pieces exist; what is missing is the lifecycle-shaped view of them.

CONSIDER, separately: refusing a gate command that is a known no-op (true, :,
exit 0) unless --no-verify is passed. --no-verify already exists and already
records the close as unverified, which is the honest way to say what
--gate true was pretending.
