---
id: ef53721ef9a3
kind: bug
title: Go grep NUL-bearing input differs from GNU binary matching in pax header checks
seq: 182
status: done
priority: p0
created: 2026-10-02T04:28:06.721935Z
assignee: codex-gpt6.1-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-02T05:22:15.973235Z
closed_by: codex-gpt6.1-sol
---

Profile D pax:185,207 are raw FAIL because the suite pipes NUL-terminated tar header fields to grep [^0-7]. Go grep returns match on NUL while GNU grep returns no match. Reproduce with public binary inputs; preserve POSIX text behavior; implement a narrow GNU-compatible binary handling rule only if justified; verify focused grep tests and targeted pax replay. Cross-reference umbrella Sprint355 3ef7ce042bac.
