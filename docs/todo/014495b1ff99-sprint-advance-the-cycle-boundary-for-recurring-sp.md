---
id: 014495b1ff99
kind: task
title: 'sprint advance: the cycle boundary for recurring sprints'
seq: 101
status: done
priority: p1
created: 2026-09-08T03:21:38.664452Z
sprint: 140
closed: 2026-09-09T01:59:19.237424Z
---

New subcommand bashy sprint advance <n>. Files the closed iteration as a committed cycle-record item, resets recurring stories (clearing Weave so weave add --from-todo works from iteration 2 on), and refuses over a running box or an open one-off. Values come from the sprint's own .bashy/sprint/<n>.advance script; the mechanism owns only the cycle number. Also fixes todo.SetStatus erasing a sprint-bound completion and an uninterpretable cadence accepted silently.
