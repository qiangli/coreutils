---
id: 38e02e398579
kind: feature
title: Implement mandatory POSIX man and gencat in Go and bind C/D base command gate
seq: 165
status: todo
priority: p0
created: 2026-10-01T05:07:21.176923Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Open Group POSIX.1-2016 Shell and Utilities base requires man and gencat. man is currently an external provider in the 116 VSC set registry; gencat is absent from that registry. Implement both in Go, register/test their command routes, and revise the C/D base command gate against the complete mandatory command inventory. Keep option-only UP/SD/LOCALEDEF commands separate from base scoring. Acceptance: official source-backed inventory; no mandatory external provider; focused Go behavior tests and suite-free readiness tests pass; exact TP denominator from valid official scenario/journal, not invented row subtraction.

2026-10-02 diagnostic update: Profile D's committed `man` set 89 has
TP4=UNRESOLVED, TP5=FAIL, TP13=FAIL, matching C's identities. TP4's suite
prerequisite hard-codes a command-count threshold greater than 109; the
configured reduced command list has exactly 109, before any `man` behavior is
tested. Record this as a configuration/authority question. TP5 exercises
`man -k -- ls`; the Go parser rejects `--`. TP13's interactive PAGER test
observes that the Go command does not invoke the configured paginator. The
agent-facing JSON descriptor may remain the documentation payload, but the
standard command-line boundary needs those behaviors if the official report
finds the assertions applicable. Resolve the base scope from the official
report rather than from the `POSIX.upe` directory label alone. Keep the
original raw codes and run focused exact TP replays after a repair.
