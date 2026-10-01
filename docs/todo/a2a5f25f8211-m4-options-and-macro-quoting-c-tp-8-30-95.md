---
id: a2a5f25f8211
kind: bug
title: 'm4 options and macro quoting: C TP 8, 30, 95'
seq: 170
status: doing
priority: p0
created: 2026-10-01T11:24:09.194281Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

C focused TP 8 rejects invalid -D macro identifiers; TP 30 exposes `changequote()` with empty parentheses restoring default quotes; ordinal TP 95 (assertion 116) exposes interspersed -s/-D/-U parsing and line synchronization. Diagnose each against POSIX.1-2017 and the licensed journal, implement focused fixes, then rerun m4. D shares the same Go multicall.
