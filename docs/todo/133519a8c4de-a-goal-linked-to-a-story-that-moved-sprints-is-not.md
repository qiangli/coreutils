---
id: 133519a8c4de
kind: task
title: A goal linked to a story that moved sprints is not reported as dangling
seq: 74
status: todo
priority: p2
created: 2026-09-06T11:30:33.81655Z
sprint: 130
---

sprintGoalDangling (pkg/weave/weave_story_goal.go) flags a linked story only
when resolveSprintStory reports Missing - i.e. when the id no longer resolves
in the store. A story that still exists but has MOVED to another sprint
resolves fine, so nothing is flagged: the goal item simply shows unchecked,
forever, with no indication why.

That is the state every carried-over sprint lands in, because moving remaining
open stories to a successor is the ordinary way to close a sprint that ran out
of time. goal add and goal link both refuse a story whose Sprint is not this
sprint, so the tool will not let you CREATE this reference - it only lets you
arrive at it, and then says nothing.

COST 2026-09-06: six goal items across sprints 123 and 126 could not close and
the checklist gave no reason. Diagnosing it meant reading
~/.bashy/sprint/queue.json directly to recover the goal-to-story links, because
no command exposes them (see the sibling story on goal-to-story visibility).

Still true after the goal rm fix: verified on sprint 130, where every carried
item reports dangling: null.

DO: treat "linked story belongs to sprint M, not this one" as a first-class
dangling reason, named in the checklist line and in the move refusal, alongside
the existing missing case. The repair verb now exists (sprint goal rm), so the
warning can point straight at it.
