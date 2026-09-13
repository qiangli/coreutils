---
id: e08516989b1e
kind: task
title: 'C3 kb context: the one budgeted assembler over rings x forms (per-ring K, resolution by budget, typed blocks, abstain=exit 0); repair recall capability ring path'
seq: 119
status: todo
priority: p0
created: 2026-09-13T01:31:31.663773Z
sprint: 163
---

Goal: kb context — the ONE budgeted assembler (the unified-graph plan's S3 seam), replacing three ad-hoc injectors that each had their own key and no budget.

bashy kb context --for "<task text>" [--files a.go,b.go] [--episode <id>] --rings repo,host,agent --forms note,page,relation,code --budget N [--min-coverage x] [--k K] --json

- Per-ring K (default 3), never cross-ring fusion; kb's BM25 + status weight + optional activation is the one ranker (already how pkg/recall renders folds as synthetic pages — keep that).
- Resolution chosen BY BUDGET: cue -> line -> full, largest that fits; never exceeds --budget (measured: cue 266 B, line 1,083 B, full 8,819 B per hit).
- Output: typed blocks {ring, form, ref (kb:<slug> | graph:<id> | code:<symbol>), tokens, text}; text mode renders bullets. Abstain below --min-coverage = empty output, exit 0. A broken ring = exit 1.
- --episode includes checkpoint-tagged agent-ring notes ONLY when named (a reconstruction is never presented as a record by default).
- Repair the pkg/recall capability ring: it opens ~/.bashy/craft but craft writes under the skills store dir (documented dead ring). Fix the path; add a test that opens real folds and gets a hit.
- code and relation forms come from C5/C6 readers; until they land, --forms code|relation is refused with the same message doctor uses for unknown forms.

Files: coreutils/pkg/kb/context.go (new; reuse Renderer{Resolution}, Search, recall.Recall), pkg/recall/cmd.go (ring path), kb.go.
Gate: go test ./pkg/kb/... ./pkg/recall/...; script/memory-eval unchanged thresholds (>=93% hit@3, >=80% abstention, <=117 tok/query) — on regression raise the evidence bar, never touch the ranker; a budget of 700 over three rings never emits more than 700 tokens (token counter = the estimator already used by PreambleBudget).
Depends on: C1.
