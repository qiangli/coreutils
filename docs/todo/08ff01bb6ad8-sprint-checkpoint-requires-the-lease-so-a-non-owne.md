---
id: 08ff01bb6ad8
kind: task
title: sprint checkpoint requires the lease, so a non-owner cannot write the resume brief
seq: 77
status: todo
priority: p2
created: 2026-09-06T11:30:33.882883Z
sprint: 130
---

sprint checkpoint refuses without the conductor lease: "sprint N lease is
unclaimed - take it explicitly before checkpointing". So the resume brief, the
one artifact a successor is told to read, can only be written by whoever has
taken accountability for delivering the sprint.

Those are different acts. Handing over what you know is not the same as
undertaking to finish the work, and coupling them means the person best placed
to write the brief - the one just finishing a pass, or setting up a successor
sprint for someone else - either takes a seat they should not hold or does not
write it.

COST 2026-09-06: sprints 129 and 130 were created to carry the open work of
five closed sprints. Full handoff briefs were written for both - traps
inherited, state of each tree, what must not be touched - and both cards still
read "continuity: (none yet - conductor: sprint checkpoint after each step)"
because the briefs had to go to the thread instead. The next manager is
directed at the continuity field and will find it empty.

DO: allow a checkpoint on a sprint with an UNCLAIMED lease, attributed to the
writer, without granting or refreshing the seat. Keep the refusal for a lease
held by SOMEONE ELSE - overwriting a live manager's brief is a different and
real hazard.
