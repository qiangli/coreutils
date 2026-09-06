---
id: c586aca38d2f
kind: task
title: sprint move enforces the goal checklist and sprint end ignores it
seq: 73
status: todo
priority: p2
created: 2026-09-06T11:30:33.793427Z
sprint: 130
---

Two closure paths answer the same question differently.

  sprint move <id> done   refuses over any unchecked goal item
                          (weave_story.go:788), a refusal --force does NOT
                          cover
  sprint end <id>         does not look at goal items at all

OBSERVED 2026-09-06: sprint 127 ended cleanly with FOUR unchecked goals
(zero-correction-round, operator-signoff, retro-defects, supported-modes-e2e),
while sprints 123 and 126 were refused by move for exactly that condition and
had to be repaired first. Same board, same hour, opposite standards.

One of the two is wrong and it is worth deciding which before either is relied
on. The argument for move's rule is that a plan you did not finish is not a
plan you may declare finished. The argument for end's silence is that end
already gates on the tree and the box, and 127's own acceptance says the
operator decides when it is done, not a checklist.

This is a DECISION, not a bug fix. Record the answer and make both paths obey
it. Do not simply copy the goal check into end without settling what closing a
sprint over an unmet plan is supposed to mean.
