---
id: 6303f4768a5b
kind: feature
title: 'pure-Go lp: POSIX lp client over IPP'
seq: 158
status: todo
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:43.964854Z
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Owner 2026-09-30: lp is base; the certified product must be pure Go. Scope (5 pt): options -c -d -m -n -o -s -t, LPDEST/PRINTER defaulting, the POSIX request-id message, exit statuses; submits via IPP Print-Job (RFC 8011) to the destination. Then the manifest drops lp for the base product. Acceptance: red/green against an in-test pure-Go IPP stub; applet-matrix shows lp as go_applet. The certification host's print destination (host service) is set up in the certification sprint.
