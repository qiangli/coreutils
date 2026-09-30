---
id: dcdaa5d5da46
kind: feature
title: 'posix-providers list: a view of the optional UP/SD external tools'
seq: 159
status: done
priority: p2
labels:
    - posix-cert
created: 2026-09-30T15:34:45.182772Z
weave: 13
assignee: qiangli
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T16:56:10.377266Z
resolution: fixed
closed_by: codex-m
---

Owner 2026-09-30: keep ex vi man ctags nm ar strip as external binmgr providers, unchanged, and list them. Scope (3 pt): 'posix-providers list' (and --json) prints each optional provider: name, POSIX option group (UP or SD), upstream, pinned version, license, platforms, provisioned on this host or not. Acceptance: red/green test on the embedded manifest; output marks them 'optional - not in the base certification claim'.
