---
id: 87582b74db2a
kind: bug
title: 'ed: preserve success exit status after stopped child in POSIX TP39'
seq: 172
status: todo
priority: p1
created: 2026-10-01T13:08:48.709981Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Fresh Profile C old-candidate POSIX08 full arm ed set (runner rc0, 671 TPs) reports ed_01 TP39 FAIL: after TSTP, the child exits 1 where the suite expects 0. Profile A GNU ed_01 does not report this blocker. Reproduce the case from the licensed journal on the disposable host, repair the pure-Go ed behavior using POSIX.1-2017 authority, add a focused regression test, and rerun the canonical ed set on C with the final candidate. Keep ed_14 TP47 UNRESOLVED as a separate disposition until its cause is known. Do not count the current C full arm as final candidate evidence.
