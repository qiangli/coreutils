---
id: 6972ef8ec6cb
kind: task
title: Map getconf:12 first divergence before correction
seq: 15
status: todo
priority: p0
created: 2026-08-31T23:33:22.016532Z
assignee: s88-getconf-hash
sprint: 88
---

Investigate getconf:12 using only public-safe metadata, Issue 7 authority,
source/history, and short native reducers. Patch only with a mapped first
divergence; otherwise retain unresolved.

2026-08-31 investigation: current Profile D records FAIL while the retained GNU
control records UNRESOLVED. The public metadata does not map this identity to a
query or first observable. Current source already provides filesystem-aware
timestamp resolution and the focused `cmds/getconf` native suite passes. No
standards-aligned product correction is justified from the numeric result.

Required redacted replay tuple: query class (system, pathname, configuration),
provider category/path/package/version/executable digest by arm, effective PATH
digest, pathname existence/type/filesystem category when applicable, numeric
exit status, stdout/stderr byte counts and digests, result phase, and first
differing observable category. No query text, output, journal text, or suite
material.
