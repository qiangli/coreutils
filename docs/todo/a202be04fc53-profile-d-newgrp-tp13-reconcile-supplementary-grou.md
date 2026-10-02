---
id: a202be04fc53
kind: bug
title: 'Profile D newgrp TP13: reconcile supplementary group order'
seq: 179
status: todo
priority: p0
created: 2026-10-02T01:04:45.926264Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Frozen Profile D newgrp:13 FAIL, same Profile C Go result; Profile B GNU provider PASS. The journal compares otherwise equal id output whose supplementary group order differs: expected 5002,8,5004; obtained 8,5002,5004. Determine whether Go newgrp credential planning, Go id formatting, or test setup changes the order. Add a focused Linux reproduction only after root cause is proven, then exact licensed TP13 replay on a pushed pinned candidate. Keep original codes.
