---
id: d953e83653dc
kind: bug
title: 'pax: classify nine Profile C POSIX failures before D'
seq: 175
status: todo
priority: p0
created: 2026-10-01T16:03:05.914181Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Profile C old-candidate POSIX08 pax set49 completed with nine FAILs at TP46,155,168,185,207,225,245,246,247. Diagnose each against journal, public POSIX semantics and controlled reproductions; separate fixture/provider causes from Go defects. Fix confirmed Go defects, run meaningful regressions and focused licensed replay with final binary before Profile D. Do not infer a single cause from the raw count.

2026-10-01 controlled diagnosis: TP225 writes successfully with `-x pax` and
global identity records, then `-r` fails while rewriting a physical USTAR
member carrying merged PAX records. This is a Go writer-format defect;
targeted Linux host reproduction passed after selecting PAX for the rewrite.
TP155 uses `-o times` and inspects each member's extended `mtime` record;
Go emitted `atime` but omitted `mtime` after truncating source nanoseconds.
The package regression now requires both records. These are two confirmed
product fixes, with the other seven still awaiting individual disposition.
