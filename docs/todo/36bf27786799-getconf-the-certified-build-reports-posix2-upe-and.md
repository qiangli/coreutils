---
id: 36bf27786799
kind: bug
title: 'getconf: the certified build reports POSIX2_UPE and POSIX2_SW_DEV as unsupported (base XCU claim without UP/SD)'
seq: 152
status: done
priority: p1
labels:
    - cert
created: 2026-09-30T15:24:38.623713Z
assignee: codex-gpt6.1-sol
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
closed: 2026-10-02T09:52:01.333047Z
closed_by: codex-gpt6.1-sol
---

Owner scope decision 2026-09-30: claim POSIX.1-2017 Shell and Utilities base; UP and SD options not claimed. Today 'getconf POSIX2_UPE' and 'getconf POSIX2_SW_DEV' report 200112 (supported) - checked via bashy on macOS 2026-09-30, verify on the Linux candidate too - which would contradict the Conformance Statement. Fix (KISS): the multicall getconf reports these two (and C-Language/Fortran development, which are not claimed either) as undefined (-1 / 'undefined') in the shipped configuration; base limits unchanged. Red/green: getconf POSIX2_UPE -> undefined; POSIX2_VERSION unchanged.

2026-10-02 Linux candidate check: on the running Profile D host,
`/vsc/cushim/getconf` points to `/vsc/sut/coreutils`; its direct output is
`undefined` for `POSIX2_UPE`, `undefined` for `POSIX2_SW_DEV`, and `200809`
for the unchanged `POSIX2_VERSION`. This is read-only observation of the
approved candidate while the licensed D arm runs.
