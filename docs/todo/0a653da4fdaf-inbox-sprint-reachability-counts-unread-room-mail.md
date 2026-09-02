---
id: 0a653da4fdaf
kind: task
title: 'inbox: sprint reachability counts unread ROOM mail, not only bus pending'
seq: 32
status: todo
priority: p1
created: 2026-09-02T18:50:40.123058Z
---

Carried out of dhnt sprint 99 (story 75c42290afa5) unimplemented. The code is here, so the todo is here.

WHAT IS WRONG. pkg/weave/weave_story_reach.go sprintOwnerUnanswered measures unanswered mail with bus.ReadPending(owner) alone. Meet room records are a separate store with their own per-reader cursors (pkg/meet SeenSeq/UnreadRecords), so a question asked in a sprint's OWN conductor room — the channel the sprint advertises — is not counted. Verified 2026-09-02: no meet/room reference anywhere in that function.

WHY IT MATTERS. This is the last surface where a sprint can report healthy over somebody waiting. The seat gate (shipped) made room mail DELIVERABLE, and the default addressee (shipped) made it ADDRESSED, but the health check still cannot see it. The original failure — four operator questions unread for five hours while the board printed a healthy owner — would now be delivered and still uncounted.

SHAPE. Add unread ROOM records for the owner's seated rooms to the same count, resolving role addresses at read time exactly as directedEvent does (a message to conductor:<n> is unread FOR the current holder). Keep it read-only: sprintOwnerUnanswered is called by a consistency check run by passers-by, and its comment already warns that SnapshotInbox would open a subscription as a side effect.

DO NOT turn this into a refusal. Reachability REPORTS; it does not gate. The rule for this whole surface is that gates exist only to guarantee delivery exactly once, and an unread message is not a reason to refuse anybody anything.

Gate: script/e2e-agent-inbox.sh check B4 is already written and PLANNED — implementing this promotes it to must-pass in the same commit.
