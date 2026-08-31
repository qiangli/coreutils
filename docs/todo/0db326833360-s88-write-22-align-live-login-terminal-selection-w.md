---
id: 0db326833360
kind: task
title: 'S88 write:22: align live login terminal selection with POSIX'
seq: 12
status: doing
priority: p0
created: 2026-08-31T23:22:13.361058Z
assignee: codex-s88-write22
sprint: 88
---

Profile D at frozen Coreutils c747cab / sh d99cc49 / Bashy 85fadd6 remains write:22=UNRESOLVED while matched Profile C passes. Prove with a narrow native reducer whether Coreutils rejects an otherwise live utmp+PTY+mesg-y recipient solely because Linux ut_pid does not own the terminal. If causal, remove only the non-standard false-negative while retaining account, live-record, device, and mesg gates; add focused native tests. No licensed journal inspection, POSIX/container/broad tests on Dragon, push, merge, or umbrella pin. Gate: focused cmds/write tests locally plus exact Novi write:22 replay by manager.
