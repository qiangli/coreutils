---
id: 31d7c9a82f44
kind: task
title: Separate chown/chgrp traversal and dereference semantics
seq: 2
status: todo
priority: p1
created: 2026-08-11T19:00:00Z
---

Repair the shared `chown`/`chgrp` recursive symlink model described in
`docs/sprint-49-salvage-reconciliation.md`. Model traversal (`P`, `H`, `L`)
separately from whether the operation affects a symlink or its referent
(`-h`/`--no-dereference` versus `--dereference`), preserving argument order.
Implement a cycle-safe logical walker so `-R -L` descends through symlinked
directories and `-R -H` follows only command-line directory symlinks.

Required tests: command-line and nested live directory links, an out-of-tree
target with an observable child, dangling links, a loop, continued traversal
after an error, and both orders of `H/L/P` and `h/dereference`. Prove new tests
red on the current base before implementation. Run focused normal/race tests,
matrix regeneration/check, and full crossvet. Do not claim a TP retirement
without current affected native evidence.
