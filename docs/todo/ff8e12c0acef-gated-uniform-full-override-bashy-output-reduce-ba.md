---
id: ff8e12c0acef
kind: task
title: 'GATED: uniform --full override + BASHY_OUTPUT_REDUCE + bashy full --'
seq: 44
status: todo
priority: p2
created: 2026-09-04T09:52:41.216938Z
sprint: 123
---

CLOSED until the Yoke gates open. Design §3.1. Register --full/--no-elide ONCE in the shared helper that already installs --json/--plain/--quiet (pkg/weave/weave.go:145) so it is uniform by construction. It beats everything including BASHY_AGENTIC. A flag cannot reach external commands on the shell path, so ship the env form and 'bashy full -- <cmd>' too. Gate: a table-driven test enumerating verbs from atlas (never a hand-maintained list).
