---
id: f29e3c7a5fd7
kind: task
title: 'fleet: ship default model/agent seeds in the embedded ring (revises Sprint 161 D5) + roster of 2026-09-13'
seq: 136
status: done
created: 2026-09-14T06:37:22.600847Z
assignee: sprint176-manager
sprint: 176
closed: 2026-09-14T06:53:23.951141Z
closed_by: sprint176-manager
---

Restore baseline/{models,agents} as ring-0 SEEDS (overwritable by shared/cloud/local), add fable5.1, gemini3.7/3.8-flash(+low), kimi-k2.7-code-highspeed, glm-5.3-flash with matching agents; fix DeepSeek id drift; tests: empty-ring test via WithBaselineFS, new TestEmbeddedSeedsResolve.
