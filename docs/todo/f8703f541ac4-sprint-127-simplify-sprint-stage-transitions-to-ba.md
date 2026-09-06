---
id: f8703f541ac4
kind: task
title: 'Sprint 127: simplify sprint stage transitions to backlog, doing, done'
seq: 67
status: done
priority: p0
created: 2026-09-06T05:48:01.169268Z
assignee: codex-sprint127
sprint: 127
closed: 2026-09-06T05:55:54.61698Z
---

Remove the unnecessary sprint-level review column as a real model change, not a display workaround. Canonical sprint columns and CLI validation/help become backlog|doing|done; review remains story/goal evidence vocabulary only. Existing persisted sprint cards in review must migrate deterministically to doing on load so no invalid legacy state survives. Update only directly affected sprint transition tests/docs/projection. Gate: go test ./pkg/weave ./pkg/webconsole and scripts/crossvet.sh; rebuild and verify the installed bashy help rejects review and advertises exactly backlog|doing|done. Commit/push coreutils, bump/commit/push bashy dependency if required, then bump/commit umbrella pins.
