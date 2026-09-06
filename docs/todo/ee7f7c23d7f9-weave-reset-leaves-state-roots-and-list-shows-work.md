---
id: ee7f7c23d7f9
kind: task
title: weave reset leaves state roots and list shows workspace-less terminal ghosts
seq: 81
status: done
priority: p1
created: 2026-09-06T11:45:30.280518Z
sprint: 130
closed: 2026-09-06T11:45:35.484239Z
---

Reset must remove all per-project weave state and the shared parent when it becomes empty. Default weave list/list --all/watch must hide killed or failed terminal records after their workspace is absent, while --history retains them. Regression tests and cross-platform gate required.
