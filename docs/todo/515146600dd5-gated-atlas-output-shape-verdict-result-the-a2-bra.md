---
id: 515146600dd5
kind: task
title: 'GATED: atlas output shape (verdict|result) — the A2 branch is invalid without it'
seq: 43
status: done
priority: p1
created: 2026-09-04T09:52:41.194334Z
sprint: 123
closed: 2026-09-06T06:14:46.241671Z
---

OPERATOR-OPENED Sprint 123 contract: classify output as the typed verdict|result vocabulary. diff, cmp, test, [, go list, and git log/status/diff/show are result-shaped; only the curated external argv[0]+argv[1] verdict table covers go build/test/vet, git push, cargo build/test, and npm test. Unknown or future shapes, commands, and subcommands default to result. Add table-driven positive, negative, and unknown coverage. Mark this story done only after focused native `go test ./pkg/atlas ./mcp` passes on dragon; Conductor owns the qualifying integration gate on novidesign.local.
