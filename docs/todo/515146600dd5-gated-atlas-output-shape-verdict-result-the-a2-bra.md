---
id: 515146600dd5
kind: task
title: 'GATED: atlas output shape (verdict|result) — the A2 branch is invalid without it'
seq: 43
status: todo
priority: p2
created: 2026-09-04T09:52:41.194334Z
sprint: 123
---

CLOSED until the Yoke gates open. MEASURED: collapsing on exit code alone gave the best reduction (97%) and the worst retention (75%) in one run, because find/grep/ls/go-list were collapsed to 200 bytes on exit 0. A verdict-shaped command's exit 0 means 'nothing to report'; a result-shaped command's output IS the answer. Add the shape to atlas.Entry (already on the exec hot path). Unknown shape MUST default to result — under-compressing is visible, discarding an answer is not.
