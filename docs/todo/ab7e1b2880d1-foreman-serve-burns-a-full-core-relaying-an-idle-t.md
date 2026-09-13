---
id: ab7e1b2880d1
kind: task
title: foreman serve burns a full core relaying an idle TUI child's PTY
seq: 127
status: todo
created: 2026-09-13T15:59:28.216066Z
sprint: 156
---

Observed 2026-09-13 15:27–16:00Z on the dev box (dhnt checkout): `bashy foreman serve sprint-165-manager` (a foreman-spawned Claude manager session) ran at 66–126% of a core for 30+ min while its child `claude` sat at 2%. The resource-observer posted host CPU pressure 100% / 92% warnings to the sprint #165 seat because of it.

Evidence: the foreman's PTY transcript (~/.bashy/foreman/<id>/log, raw escapes) grew ~590 B/s while the child only drew its TUI spinner (5,600+ cursor show/hide toggles in 31 min). Relaying ~600 B/s should be ~0% CPU; the cost is per write, not per byte — suspect a full-buffer re-parse/redraw or a busy poll on the pty read side. Same family as the Sprint 138 watcher-CPU incident (the permanent repair was left open there). Binary is stripped, so `sample` shows no frames; reproduce with a managed session that runs an interactive TUI child and pprof `foreman serve`.

Acceptance: `foreman serve` hosting an idle TUI child stays under 2% CPU (measure with `ps -o pcpu` over 60 s); the transcript is still complete.
