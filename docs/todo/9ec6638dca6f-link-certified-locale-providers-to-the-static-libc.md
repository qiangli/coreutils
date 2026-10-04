---
id: 9ec6638dca6f
kind: bug
title: Link certified locale providers to the static libc and remove secondary dlopen crash
seq: 183
status: done
priority: p0
created: 2026-10-03T04:10:25.257079Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:04.329484Z
closed_by: codex-gpt6-sol
---

One-file Linux CGO1 Bashy intermittently SIGSEGVs in glibc read_alias_file on de_DE.iso88591 awk locale opens. GDB core shows static Bashy plus separately dlopened libc.so.6 via Coreutils purego. Provide cert-tag linked libc locale bindings for ctype/collate, keep noncert purego path, and verify repeated cold awk plus focused VSC IC639/640 and full awk set under POSIX mode.

## Sprint 355 acceptance evidence 2026-10-04

Linked-libc locale repair is merged at b24d3a76 and 3dedce4a. The guarded full6 static candidate completed awk and all 117 sets with zero caps or new failures; original crash route is absent on the approved build. Historical raw journals and any pending formal certification decisions are unchanged.
