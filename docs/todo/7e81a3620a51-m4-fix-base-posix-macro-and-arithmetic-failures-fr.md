---
id: 7e81a3620a51
kind: bug
title: 'm4: fix base POSIX macro and arithmetic failures from C baseline'
seq: 167
status: doing
priority: p0
created: 2026-10-01T11:21:51.882616Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Focused licensed Profile C m4 set on 2026-10-01: 101 TPs, 9 FAIL and 1 UNRESOLVED. Investigate TP 8, 30, 47, 48, 51, 61, 76, 95, 97; fix Go m4 semantics on frozen source, run focused assertions, then rerun licensed m4 on an ephemeral host. Preserve TP numbers and raw evidence privately; do not copy VSC text into this open-source story. D uses the same Go utility binary, so these are potential base claim blockers.
