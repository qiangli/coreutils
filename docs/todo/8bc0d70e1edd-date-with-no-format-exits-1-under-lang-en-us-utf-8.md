---
id: 8bc0d70e1edd
kind: bug
title: date with no format exits 1 under LANG=en_US.UTF-8 (LC_TIME locale unavailable); must fall back to C/POSIX like GNU
seq: 187
status: todo
priority: p1
labels:
    - release
created: 2026-10-23T17:34:58.538718Z
---

Found 2026-10-23 (Sprint 458 session) on macOS with bashy 5.3.0(1)-bashy-dev (ecf0805): plain 'date' and 'date -u' print 'date: LC_TIME locale "en_US.UTF-8" is unavailable: embedded date data is limited to C/POSIX and de_DE' and exit 1. 'date -u +%H:%M' works. Earlier umbrella todo 4763d1f4 (marked done) covered a similar case; this is a remaining path (default format). Expected: an unavailable locale falls back to C/POSIX output, as GNU does (setlocale failure is not fatal). Triage: confirm no POSIX/VSC impact (cert runs under C), then schedule. Red/green: LANG=en_US.UTF-8 date exits 0 with C-format output.
