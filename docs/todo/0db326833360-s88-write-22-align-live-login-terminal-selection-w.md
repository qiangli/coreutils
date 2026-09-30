---
id: 0db326833360
kind: task
title: 'S88 write:22: align live login terminal selection with POSIX'
seq: 12
status: doing
priority: p0
created: 2026-08-31T23:22:13.361058Z
assignee: codex-s88-write22
sprint: 100
---

Profile D at frozen Coreutils c747cab / sh d99cc49 / Bashy 85fadd6 remains write:22=UNRESOLVED while matched Profile C passes. Prove with a narrow native reducer whether Coreutils rejects an otherwise live utmp+PTY+mesg-y recipient solely because Linux ut_pid does not own the terminal. If causal, remove only the non-standard false-negative while retaining account, live-record, device, and mesg gates; add focused native tests. No licensed journal inspection, POSIX/container/broad tests on Dragon, push, merge, or umbrella pin. Gate: focused cmds/write tests locally plus exact Novi write:22 replay by manager.

## Review 2026-09-30 (steward)

- Status: product fix already integrated - coreutils 7873a9a0 ("accept live recorded terminal sessions", 2026-08-31) is an ancestor of today's head dc805500. Only the exact licensed replay is missing. Status still says `doing` with a stale assignee seat (codex-s88-write22); no worker is on it.
- Outdated: frozen pins "Coreutils c747cab / sh d99cc49 / Bashy 85fadd6" and the named replay host; the old replay host routing is contextual and no longer applies.
- Next step: no code work. Take write:22's verdict from the fresh baseline full arm at the frozen candidate (Sprint 110 f093f2d6bba7) instead of a separate single-TP replay; run a focused exact replay only if that arm is non-PASS.
- Acceptance: write:22 PASS in the baseline arm (or an exact replay) at a recorded candidate digest; focused `go test ./cmds/write` green on Linux.
- Depends on: Sprint 110 baseline arm. Done-candidate once that evidence exists.
