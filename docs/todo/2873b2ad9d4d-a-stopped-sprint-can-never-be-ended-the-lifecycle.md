---
id: 2873b2ad9d4d
kind: task
title: 'A stopped sprint can never be ended: the lifecycle has no stopped to ended edge'
seq: 72
status: todo
priority: p1
created: 2026-09-06T11:30:33.769965Z
sprint: 130
---

pkg/weave/weave_story_box.go:505 refuses end when a box exists but none is
running: "sprint N has no running box - sprint start N opens one". The unboxed
escape hatch beside it is narrower than it looks - unboxedEnd is
(ending AND len(s.Boxes) == 0) - so it covers a sprint that NEVER had a clock,
and not one whose clock was stopped.

The result is that the lifecycle has no stopped -> ended edge. A sprint stopped
cleanly, gate green, seat released - the state a careful manager leaves behind -
can never be ended. The only routes are to start a new time box purely so it can
be closed, which fabricates a box that held no work, or to bypass end entirely
with sprint move.

OBSERVED 2026-09-06 on sprint 126. Its previous manager had stopped it
gate-green and released the seat, deliberately. end refused; the close finally
went through move, so end's repo and worker verification never ran on it at all.

DO: let end accept a stopped box - close the lifecycle without reopening or
inventing a clock, and run every other end check unchanged. The distinction
end actually needs is "is this sprint still running work", not "is a clock
ticking".

DO NOT close this by telling the manager to start a box first. That is the
current advice, and taking it writes a false record.
