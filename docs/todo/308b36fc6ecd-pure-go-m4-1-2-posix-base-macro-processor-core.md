---
id: 308b36fc6ecd
kind: feature
title: 'pure-Go m4 (1/2): POSIX base macro processor core'
seq: 153
status: todo
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:37.91961Z
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Owner 2026-09-30: m4 is base in POSIX.1-2017 (moved from XSI to Base) and the certified product must be pure Go. Scope (8 pt): tokenizer, quoting (changequote), comments (changecom), argument collection and rescanning; builtins define undefine defn pushdef popdef ifdef ifelse shift dnl len index substr translit incr decr eval; options -s -D -U. Acceptance: red/green unit tests per builtin; a differential test against the current external GNU m4 1.4.19 provider on the POSIX examples; no GNU extensions required.
