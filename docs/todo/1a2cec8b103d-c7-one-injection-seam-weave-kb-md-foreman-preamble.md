---
id: 1a2cec8b103d
kind: task
title: 'C7 one injection seam: weave KB.md, foreman preamble, recall.PreambleForHost all call kb context (rings repo,host) behind the BASHY_KNOWLEDGE control arm'
seq: 123
status: todo
priority: p2
created: 2026-09-13T01:31:31.776332Z
sprint: 163
---

Goal: one injection seam. The three shipped injectors each have their own key and no budget; they all become calls to kb context.

- coreutils/pkg/weave/weave_kb.go: KB.md at workspace spawn = kb context --for "<issue title>" --rings repo,host --forms note,page --budget <existing 400-rune-body-equivalent> (closes: repo docs/kb never reaches weave workers because the writer was host-only).
- coreutils/pkg/foreman/kb.go preamble and coreutils/pkg/recall/inject.go PreambleForHost (bashy chat launch) call the same function with PreambleBudget 700.
- BASHY_KNOWLEDGE stays the control arm of the running experiment — do NOT flip its default; the seam change is behind it exactly as today.
- Remove the duplicated Terms()/render code paths once all three call kb context; keep the Diagnose report on no-match for weave.

Files: the three injectors above + tests (weave_kb_test.go, recall/inject_test.go, foreman tests).
Gate: go test ./pkg/weave/... ./pkg/foreman/... ./pkg/recall/...; a repo-ring page reaches a weave KB.md; preamble bytes never exceed the budget; with BASHY_KNOWLEDGE unset behaviour is unchanged.
Depends on: C3.
