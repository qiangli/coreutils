---
id: 43125aa072eb
kind: task
title: 'BUG: BASHY_HOME does not isolate the room/mb board, so sandboxed runs post to the real host board'
seq: 102
status: todo
priority: p1
created: 2026-09-08T03:44:07.02766Z
sprint: 140
---

FOUND while dogfooding sprint advance: two smoke runs under an isolated BASHY_HOME each announced "sprint #1 release train — created in backlog" onto the OPERATOR'S REAL message board (mb entries 570 and 571). The sprint store was correctly sandboxed; the announce was not.

ROOT CAUSE — two isolation ladders that disagree:

  weave/weave_story.go sprintStoreDir():
    BASHY_SPRINT_DIR -> $BASHY_HOME/sprint -> ~/.bashy/sprint
  room/room.go Dir() (line 205):
    BASHY_ROOM_DIR -> ~/.bashy/room          <-- no BASHY_HOME rung

sprintStoreDir documents BASHY_HOME as "the whole bashy home relocated (tests, sandboxed runs)". room.Dir does not honour it, so relocating the bashy home sandboxes the sprint board while every announce, mb post and bus event still lands on the shared host board.

WHY IT MATTERS: BASHY_HOME is the knob an agent reaches for to run a safe test, and it is documented as relocating the WHOLE home. A sandbox that silently leaks into shared state is worse than no sandbox, because the caller believes they are isolated. This is the same class as the earlier defect where bashy inbox tests drove a ghost conductor in the operator|s real sprint store.

FIX: give room.Dir the same middle rung — BASHY_ROOM_DIR -> $BASHY_HOME/room -> ~/.bashy/room. Then audit every other store for the same gap (kb, bus, fleet, skills, execlog) and add a test asserting that setting BASHY_HOME alone relocates all of them.

NOT FIXED HERE: mb is append-only by design, so entries 570 and 571 cannot be removed. A board note explains them.
