---
id: 5ef807881811
kind: feature
title: 'pure-Go localedef (3/3): LC_COLLATE from localedef sources drives pkg/collate'
seq: 157
status: todo
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:42.775348Z
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Scope (8 pt): collating elements, symbols and order from the compiled locale feed pkg/collate, so sort, ls, comm, join and bracket expressions honour a locale built by our localedef. Then the posixprovider manifest drops localedef for the base product. Acceptance: red/green on a custom collation order; applet-matrix shows localedef as go_applet.
