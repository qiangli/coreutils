---
id: cddd43fd2350
kind: task
title: Classify m4 TP 101 empty symlink setup failure for Linux
seq: 168
status: todo
priority: p0
created: 2026-10-01T11:21:51.931421Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Focused C m4 TP 101 is UNRESOLVED before exercising m4 because the licensed GA42 helper attempts ln -s with an empty target. On the pinned Ubuntu host both /usr/bin/ln and Bashy Go ln exit 1 for the empty target; Linux symlink(2) rejects it. Investigate official VSC disposition or a sanctioned harness configuration remedy. Do not change Go ln merely to claim a kernel-impossible symlink; retain the raw blocker until authoritative disposition.
