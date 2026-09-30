---
id: 1ecc71124d47
kind: feature
title: 'pure-Go m4 (2/2): diversions, files, system, exit; switch the multicall to the Go applet'
seq: 154
status: assigned
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:39.171598Z
assignee: s340-m4-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Scope (5 pt): divert undivert divnum, include sinclude, syscmd sysval, maketemp mkstemp, m4exit m4wrap, errprint dumpdef traceon traceoff. Then the multicall dispatches m4 to the Go applet and the posixprovider manifest drops m4 for the base product. Acceptance: red/green tests; differential vs GNU m4 on the same corpus; applet-matrix shows m4 as go_applet.
