---
id: 4dc73dbaebbe
kind: bug
title: 'm4 mkstemp: preserve trailing X template width'
seq: 171
status: doing
priority: p0
created: 2026-10-01T11:24:09.242238Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

C focused TP 97: Go os.CreateTemp leaves earlier X bytes and appends its own random suffix, so m4 mkstemp does not replace the exact count of trailing X bytes. Use secure exclusive creation with the requested suffix width, test 6 and 8 X cases, and rerun licensed m4. D shares the same Go multicall.
