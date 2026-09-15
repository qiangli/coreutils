---
id: a754705c3c7e
kind: task
title: S183.misc — make deterministic gates the default in bashy weave
seq: 143
status: assigned
priority: p0
created: 2026-09-15T01:13:30.97491Z
assignee: codex-gpt5.6-sol
sprint: 183
---

Emergency fix: change weave run verifiability default from implicit judge=required to judge=none. Deterministic verify/suite gates are sufficient unless the caller explicitly opts into --judge required or --review-agent. Update help, normalization, and focused tests. Preserve explicit required behavior and salvage --no-review semantics. No model judge for this story.
