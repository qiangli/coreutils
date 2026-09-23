---
id: cd0ca405c595
kind: bug
title: S253.8 coreutils nested owned-child argv/env handoff
seq: 151
status: doing
priority: p1
labels:
    - windows
    - parity
created: 2026-09-23T02:32:14.248617Z
assignee: codex-gpt5.6-sol
sprint: 253
sprint_id: d25e93b0-ad03-56f2-831f-1e9f626e609c
sprint_title: 'Windows fixtures 76/86 to done: one regression, four found causes, one probe, one provisioning'
---

Bashy shell ownedexec transport is merged, but nested coreutils applets xargs/env/nice/timeout and other direct launchers still use native exec and Windows cannot pass >32K argv. Add verified Bashy-owned bounded handoff at coreutils launch boundary using shared sh/interp/ownedexec framing; preserve argv0/environment, Unix exec PID, inherited descriptors, and native behavior for unowned targets. Gate with focused Windows and Unix proof, then manager reviews and merges.
