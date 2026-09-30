---
id: 9f8ce58c4dd7
kind: feature
title: 'pure-Go localedef (2/3): compile to a Go locale store read by pkg/locale and the locale applet'
seq: 156
status: todo
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:41.569637Z
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Scope (8 pt): localedef writes a compiled locale into a Go-owned locale directory (LOCPATH-style, KISS format); pkg/locale and the locale applet resolve LANG/LC_* to it (LC_CTYPE classes, LC_NUMERIC, LC_MONETARY, LC_TIME, LC_MESSAGES). Existing host-locale behaviour stays for locales not compiled by us. Acceptance: red/green: define a locale with a distinctive decimal point and yes/no expressions, then 'locale -k' and one consuming utility reflect it.
