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
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Owner scope decision 2026-09-30: claim POSIX.1-2017 Shell and Utilities base; UP and SD options not claimed. Today 'getconf POSIX2_UPE' and 'getconf POSIX2_SW_DEV' report 200112 (supported) - checked via bashy on macOS 2026-09-30, verify on the Linux candidate too - which would contradict the Conformance Statement. Fix (KISS): the multicall getconf reports these two (and C-Language/Fortran development, which are not claimed either) as undefined (-1 / 'undefined') in the shipped configuration; base limits unchanged. Red/green: getconf POSIX2_UPE -> undefined; POSIX2_VERSION unchanged.
