---
id: 4eda8e59ce5d
kind: task
title: 'S2: todo add/edit --note - reads stdin'
seq: 142
status: done
priority: p1
created: 2026-09-14T20:26:07.028985Z
assignee: joist
sprint: 180
closed: 2026-09-14T20:31:12.871757Z
closed_by: joist
---

In coreutils/pkg/todo/cli.go: when --note is exactly '-', read the body from cmd.InOrStdin() for both add and edit (the convention skill add / agent add already use). One helper, two call sites, one test. Gate: go test ./pkg/todo -run NoteStdin
