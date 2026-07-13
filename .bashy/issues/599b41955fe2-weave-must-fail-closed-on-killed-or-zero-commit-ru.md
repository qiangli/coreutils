---
id: 599b41955fe2
kind: bug
title: Weave must fail closed on killed or zero-commit runs
status: triaged
stage: code
priority: p0
refs:
    - ../bashy
reporter: qiangli
created: 2026-07-13T18:09:46.60601Z
---

The bashy register tracks 8d598429: a watchdog-killed run with exit zero and zero commits can become submitted, then an empty pull can close its issue as fixed. Implement evidence-carrying terminal transitions in coreutils pkg/weave: KilledBy must prevent submitted regardless of exit code; zero commits must not be mergeable; pull must refuse empty branches; register closure must require an actual merged diff. Add focused regression tests. Keep the public behavior brand-neutral. Required evidence: targeted pkg/weave tests and the canonical coreutils non-external test scope. Commit all work.
