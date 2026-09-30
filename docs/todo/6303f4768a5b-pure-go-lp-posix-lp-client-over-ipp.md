---
id: 6303f4768a5b
kind: feature
title: 'pure-Go lp: POSIX lp client over IPP'
seq: 158
status: assigned
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:43.964854Z
weave: 12
assignee: claude-opus5.5
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
resolution: fixed
closed_by: claude-l
---

Owner 2026-09-30: lp is base; the certified product must be pure Go. Scope (5 pt): options -c -d -m -n -o -s -t, LPDEST/PRINTER defaulting, the POSIX request-id message, exit statuses; submits via IPP Print-Job (RFC 8011) to the destination. Then the manifest drops lp for the base product. Acceptance: red/green against an in-test pure-Go IPP stub; applet-matrix shows lp as go_applet. The certification host's print destination (host service) is set up in the certification sprint.

Conductor 2026-09-30: code merged (coreutils #12 claude-sonnet5.5 ef9c8b5a + rework 750d216f, merge a3d70fbd; -m via RFC 3995 subscription, one request per invocation, 7 tests). REOPENED for the remaining acceptance: switch-over (cmds/all import, required-commands owner row, applet-matrix regen, manifest drops lp).
