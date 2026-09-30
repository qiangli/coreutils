---
id: 36bf27786799
kind: bug
title: 'getconf: the certified build reports POSIX2_UPE and POSIX2_SW_DEV as unsupported (base XCU claim without UP/SD)'
seq: 152
status: todo
priority: p1
labels:
    - cert
created: 2026-09-30T15:24:38.623713Z
sprint: 100
sprint_id: b25f503b-9786-5052-8c12-472beb844ecb
sprint_title: Sprint 100 — Profile D residual closure and final certification rerun
---

Owner scope decision 2026-09-30: claim POSIX.1-2017 Shell and Utilities base; UP and SD options not claimed. Today 'getconf POSIX2_UPE' and 'getconf POSIX2_SW_DEV' report 200112 (supported) - checked via bashy on macOS 2026-09-30, verify on the Linux candidate too - which would contradict the Conformance Statement. Fix (KISS): the multicall getconf reports these two (and C-Language/Fortran development, which are not claimed either) as undefined (-1 / 'undefined') in the shipped configuration; base limits unchanged. Red/green: getconf POSIX2_UPE -> undefined; POSIX2_VERSION unchanged.
