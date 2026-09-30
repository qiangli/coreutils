---
id: 5a9e29f34671
kind: feature
title: 'pure-Go localedef (1/3): parse POSIX charmap and locale definition sources'
seq: 155
status: done
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:40.386122Z
weave: 11
assignee: qiangli
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T16:57:32.831871Z
resolution: fixed
closed_by: codex-k
---

Owner 2026-09-30: localedef is base; the certified product must be pure Go. Scope (5 pt): charmap files and locale source (LC_CTYPE LC_COLLATE LC_MONETARY LC_NUMERIC LC_TIME LC_MESSAGES, copy, escape_char/comment_char, ellipses), options -c -f -i -u, POSIX exit statuses and diagnostics. Acceptance: red/green parser tests on POSIX example sources and one real glibc source (e.g. en_US + UTF-8 charmap).
