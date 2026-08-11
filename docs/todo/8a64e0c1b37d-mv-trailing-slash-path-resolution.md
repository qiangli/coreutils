---
id: 8a64e0c1b37d
kind: task
title: Preserve mv trailing-slash pathname semantics without breaking -T
seq: 3
status: todo
priority: p2
created: 2026-08-11T19:00:00Z
---

Fix the `mv` raw-operand/path-normalization gap described in
`docs/sprint-49-salvage-reconciliation.md`. A destination ending in `/` must
resolve to a directory before the normalized `RunContext.Path` value erases
that syntax, but this validation must remain separate from GNU `-T`
(`--no-target-directory`). Report a missing component distinctly from an
existing non-directory, preserve every source on failure, and retain
multi-source and `--strip-trailing-slashes` behavior.

Required tests: existing regular file, nonexistent path, real directory,
symlink to directory, `-T` with a directory operand, multiple sources, and the
explicit stripping extension. Prove the behavioral tests red on the current
base, then run focused normal/race tests, matrix regeneration/check, and full
crossvet. Do not infer a TP identity or retirement from numeric ordering.
