---
id: 27ae4f3792e2
kind: task
title: The managed-inbox gate is on sprint focus, the one verb that changes nothing
seq: 99
status: todo
priority: p2
created: 2026-09-07T20:29:00.658455Z
sprint: 136
---

MEASURED 2026-09-07 while conducting sprint #135 as owner `mullion`:

  bashy sprint focus 136 86a6cc12 --repo <root>
    exit=1
    sprint focus: sprint owner mullion has no verified managed inbox delivery;
    launch it through Bashy (a terminal `inbox --watch` alone cannot wake the agent)

THE CHECK IS SOUND. sprintInboxDeliveryLive asks whether mail addressed to this owner can actually WAKE it, and an owner nothing can push to is a real problem — sprint #130 already carries c8bb9009078d, "sprint take accepts an owner nothing can push to". Nothing here argues the check should go away.

IT IS ON THE WRONG VERB, and that is the defect. grep says sprintInboxDeliveryLive has exactly two callers: weave_story_reach.go:302, where it is a REPORT field, and weave_story_goal.go:615, where it REFUSES `sprint focus`. Every other verb proceeds without it. In one session the same owner successfully ran:

  sprint start 135 --owner mullion      seated the manager and started the clock
  sprint take 135 --owner mullion       claimed the conductor lease, twice
  sprint checkpoint 135                 wrote the continuity record
  sprint end 135 --gate '<cmd>'         ENDED THE SPRINT

and was refused only by `sprint focus`, which sets an advisory pointer at the story the manager intends to work next. So the gate sits on the cheapest, most reversible, purely-bookkeeping operation while every consequential one — including `end`, which is irreversible and closes the card — lets the identical owner through.

THE CONSEQUENCE IS NOT AN INCONVENIENCE. It teaches the wrong lesson. An operator or agent that hits this concludes the seat is not properly established and goes looking for the problem, when in fact the sprint is fully operable and only one advisory verb disagrees. It cost exactly that during #135: focus was abandoned and the sprint was driven without it.

WANTED — decide which of these is true, and make the code say it:
  (a) The check belongs on the verbs that SEAT an owner (start / take / edit --owner), where an unwakeable owner is actually consequential. Then focus inherits it for free, and c8bb9009078d on #130 is the same fix — coordinate rather than duplicate.
  (b) The check belongs nowhere as a REFUSAL and everywhere as a WARNING, because a human operator driving a sprint from a terminal is a legitimate mode that no managed session backs. `sprint reach` already reports it this way.
  (c) It belongs on focus specifically for a reason nobody has written down. If so, write it down — right now the asymmetry reads as an accident, and the next person will "fix" it by deleting the check.

DO NOT resolve this by deleting the check from focus and moving on. That trades a visible wrong-verb error for a silent gap, and the underlying question — can this owner be reached at all — stays unanswered on the verbs where it matters.

GATE. A test that pins the DECISION rather than the current placement: for whichever of (a)/(b)/(c) is chosen, assert that an owner with no live delivery is treated the same way by seating, focusing and ending — either all refuse, all warn, or the asymmetry is asserted together with the recorded reason for it.
