---
id: fc112dc9d7fb
kind: bug
title: Correct weave terminal-evidence patch for reassigned runs
status: closed
stage: code
priority: p0
refs:
    - ../bashy
reporter: qiangli
created: 2026-07-13T18:19:05.316293Z
closed: 2026-07-13T18:29:54.595162Z
resolution: fixed
closed_by: codex-gpt-5.5-i
---

Corrective review of coreutils weave run 86 commit ee74a11. Import that commit from /Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-86, then fix the live counterexample: a run killed under agy was relaunched under codex; state became working but stale KilledBy remained. The committed pull guard would reject a later successful reassignment. On every legitimate relaunch, clear current-run terminal evidence including KilledBy, exit code, finished time, head and verification fields as appropriate, while preserving historical facts in the append-only comments. Add an end-to-end regression test proving killed run to relaunch to fresh committed submission is mergeable and that truly killed or zero-commit runs remain fail-closed. Run pkg/weave tests and commit the complete corrected series.
