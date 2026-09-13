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

Goal: one injection seam. The three shipped injectors each have their own key and no budget; they all become calls to the assembler. Design of record: dhnt docs/kb-rings-forms-stages.md §5, §6, §9; plan D1/D2.

- pkg/recall: replace PreambleForHost with Context(goal, opts) — the same code path `kb context` runs (C3), returning the rendered preamble under a budget. This is the one function; the three callers below hold no ranking or rendering of their own.
- coreutils/pkg/weave/weave_kb.go: KB.md at workspace spawn = recall.Context(for: "<issue title>", rings: repo,host, forms: note,page, budget: the existing 400-rune-body-equivalent) — closes "repo docs/kb never reaches weave workers" (the writer was host-only). Keep the Diagnose report on no-match for weave.
- coreutils/pkg/foreman/kb.go preamble and pkg/chat/chat.go (~line 1010, the BASHY_KNOWLEDGE treatment arm) call the same function with PreambleBudget 700.
- BASHY_KNOWLEDGE stays the control arm of the running experiment — do NOT flip its default; the seam change is behind it exactly as today.
- Gate-event writer (plan D2): weave's drain gate (pkg/weave/weave_story_drain.go runDrainGate) appends the gate verdict as a kb observe --kind gate event (ran/passed/command/exit_code/where/episode) through the kb package API, so `kb validate --from-gate <id>` has something real to resolve. Record the event id in the run's observation (pkg/weave/memory/observation.go) so it is discoverable.
- Remove the duplicated Terms()/render code paths once all three callers go through Context.

Files: pkg/recall/inject.go, pkg/weave/{weave_kb.go,weave_story_drain.go}, pkg/weave/memory/observation.go (one field), pkg/foreman/kb.go, pkg/chat/chat.go + tests (weave_kb_test.go, recall/inject_test.go, foreman tests).
Gate: go test ./pkg/weave/... ./pkg/foreman/... ./pkg/recall/... ./pkg/chat/...; a repo-ring page reaches a weave KB.md; preamble bytes never exceed the budget; with BASHY_KNOWLEDGE unset behaviour is unchanged; a drain gate run writes one kind=gate journal event whose id validate --from-gate accepts when passed.
Depends on: C3 (Context), C4 (observe event).
