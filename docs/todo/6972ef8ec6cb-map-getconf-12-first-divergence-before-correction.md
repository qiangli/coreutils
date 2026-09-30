---
id: 6972ef8ec6cb
kind: task
title: Map getconf:12 first divergence before correction
seq: 15
status: wontfix
priority: p0
created: 2026-08-31T23:33:22.016532Z
assignee: s88-getconf-hash
closed: 2026-09-30T15:33:55.149347Z
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

## Review 2026-09-30 (steward)

- Status: unknown at today's pins. The FAIL was measured at Sprint 85/88 pins. getconf and its inputs changed since: 799668de (compile-time constants answered on all platforms), 9175655a (effective Bashy ARG_MAX), 753cdd0f/b00181f5 (exec budget), cbf37428 (locale/ARG_MAX test alignment) - any of these may move getconf:12.
- Outdated: "current Profile D" and "retained GNU control" refer to Sprint 85/88; assignee seat s88-getconf-hash is gone.
- Next step: do NOT patch. Take getconf:12 from the fresh baseline full arm at the frozen candidate (Sprint 110 f093f2d6bba7). If still FAIL while the GNU control is not PASS either, request the redacted tuple above; map the first divergence before any change.
- Acceptance: PASS at the frozen candidate, or a mapped first divergence + focused native test + exact replay PASS, or an authoritative disposition backed by the GNU control.
- Depends on: Sprint 110 baseline arm.
