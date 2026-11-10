---
id: 8531f9377261
kind: bug
title: tar -x without -f does not read stdin (GNU default)
seq: 189
status: todo
priority: p2
labels:
    - tar
    - compat
created: 2026-11-10T12:01:25.293113Z
---

'git archive HEAD | tar -x -C DIR' fails in bashy with 'tar: no archive file specified; use -f FILE'. GNU tar (and bsdtar) default the archive to stdin when -f is absent and stdin is not a terminal. Drop-in compatibility: default to '-' in that case; red/green test.
