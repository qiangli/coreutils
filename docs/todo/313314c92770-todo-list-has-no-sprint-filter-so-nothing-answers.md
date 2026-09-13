---
id: 313314c92770
kind: task
title: todo list has no --sprint filter, so nothing answers what is still open on a sprint
seq: 75
status: todo
priority: p1
created: 2026-09-06T11:30:33.838279Z
sprint: 130
---

"What is still open on this sprint" is the question a closing manager asks
first, and there is no command that answers it. bashy todo list --sprint N
exits 1 with "unknown flag: --sprint".

COST 2026-09-06, paid twice: closing five sprints meant dumping three repo
stores with todo list --json and filtering on the sprint field in Python, once
to find the still-linked open stories and again to find the stories that had
been unlinked but were still referenced by unmet goals. The board knows the
answer; the CLI will not project it.

Note the history: sprint 127's seat retro raised "todo list --sprint silently
empties" and RETRACTED it after finding the silence was a stderr redirect in
the reporter's own wrapper. The retraction was correct about the noise and
wrong about the conclusion - the flag does not exist at all, and the loud
error is what you get.

DO: add --sprint N to todo list, resolving across the sprint's tracked story
roots the way loadSprintStories already does, so one command answers it for
every repo the sprint owns rather than one repo at a time.
