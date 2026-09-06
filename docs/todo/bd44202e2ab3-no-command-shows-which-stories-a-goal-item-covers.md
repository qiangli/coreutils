---
id: bd44202e2ab3
kind: task
title: No command shows which stories a goal item covers
seq: 76
status: todo
priority: p2
created: 2026-09-06T11:30:33.860876Z
sprint: 130
---

Nothing in the CLI shows which stories a goal item covers.

  sprint show          renders the checklist as mark + id + text only
  sprint show --json   goal_progress entries are {checked, dangling, id}

The links exist and are durable - sprintGoalItem.Stories, persisted in the
queue - and every question about a stuck checklist is a question about them:
which story is holding this item open, which repo is it in, what is its status.

COST 2026-09-06: recovering the goal-to-story map for sprints 122, 123, 126 and
127 meant reading ~/.bashy/sprint/queue.json and walking it in Python, then
cross-reading each story file for its status. That is reaching behind the tool
that owns the state, which is exactly what a read-only projection should have
made unnecessary.

DO: include the resolved story refs (repo, id, status, priority) in
goal_progress, and render them under an unchecked item in the text view -
unchecked items only, so a satisfied checklist stays one line per outcome. The
resolver already exists (resolveSprintStory) and the renderer already calls it
for the dangling warning.
