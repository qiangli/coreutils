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
