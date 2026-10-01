---
id: 9fc8a1fffd4d
kind: bug
title: 'getconf: keep timestamp resolution stable for near-PATH_MAX relative operand'
seq: 173
status: todo
priority: p1
created: 2026-10-01T13:28:31.572047Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Profile C POSIX08 getconf set 27 TP12 FAIL: near-PATH_MAX relative pathname returned undefined where the short pathname returned 1 for _POSIX_TIMESTAMP_RESOLUTION. Reproduce with licensed journal on C, repair mountpoint detection without materializing an overlong absolute pathname, add focused regression, and rerun getconf with final candidate. Compare A control before classifying final applicability.
