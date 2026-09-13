---
id: e08516989b1e
kind: task
title: 'C3 kb context: the one budgeted assembler over rings x forms (per-ring K, resolution by budget, typed blocks, abstain=exit 0); repair recall capability ring path'
seq: 119
status: assigned
priority: p0
created: 2026-09-13T01:31:31.663773Z
weave: 15
assignee: qiangli
sprint: 163
---

Goal: kb context — the ONE budgeted assembler (the unified-graph plan's S3 seam), replacing three ad-hoc injectors that each had their own key and no budget. Design of record: dhnt docs/kb-rings-forms-stages.md §5, §7, §8; plan D1/D4/D6.

WHERE IT LIVES (plan D1 — corrected from the original brief): pkg/recall, NOT pkg/kb. pkg/kb is pinned as an import leaf (pkg/fleet/leaf_test.go: scope, bus, git only) and pkg/recall already imports kb (HostRing wraps kb.Store), so "pkg/kb/context.go reusing recall.Recall" is an import cycle. pkg/recall is already the cross-ring composer (Reader, Query{K,Budget}, RingError, Rings(), PreambleBudget) — extend it. bashy mounts it as `bashy kb context` beside `kb recall` via internal/agentos/kbrecall.go (B1).

bashy kb context --for "<task text>" [--files a.go,b.go] [--episode <id>] --rings repo,host,agent --forms note,page,relation,code --budget N [--min-coverage x] [--k K] --json

- Rings: add RepoRing (the repo's committed docs/kb/ — recall reads ~/.bashy/kb today and NEVER docs/kb/, which is why repo kb never reaches a weave worker) and AgentRing (<agent-data>/kb/, owner-only, via the KindAgent resolution C1 adds to pkg/scope) beside the existing HostRing and CapabilityRing. Precedence agent > repo > host for presentation; per-ring K (default 3), never cross-ring fusion; kb's BM25 + status weight + optional activation is the one ranker.
- Resolution chosen BY BUDGET: cue -> line -> full, the largest that fits (measured: cue 266 B, line 1,083 B, full 8,819 B per hit); never exceeds --budget (token counter = the estimator PreambleBudget already uses).
- Output: the FROZEN envelope dhnt docs/kb-context-envelope.json — copy it byte-identical to pkg/recall/testdata/kb-context-envelope.json and test against it: blocks[]{ring,form,ref (kb:<slug>|graph:<id>|code:<id>),tokens,text} + optional resolution/status/why/source; budget{limit,used}; abstained; rings[]{name,ok,error?}. Text mode renders bullets. Abstain below --min-coverage = empty blocks, exit 0. A ring with ok:false = exit 1 WITH THE PATH IT OPENED (this closes sprint 89's open "substrate-reach" story: honest empty, one resolver — close that story as MOVED -> 163/C3 when this lands; plan D6).
- --episode includes checkpoint-tagged agent-ring notes ONLY when named.
- Repair the capability ring: pkg/recall/cmd.go opens ~/.bashy/craft but craft writes folds under the skills store dir (pkg/craft/cmd.go WithStoreDir default ~/.config/bashy/skills). Fix the path; add a test that opens real folds and gets a hit.
- code and relation forms come from readers that land later (C5 RelationRing in pkg/kb; C6 CodeRing in cmds/graph injected at the mount) — until they land, --forms code|relation is refused with the same message doctor uses for unknown forms. Expose a Reader-injection point so the mount can add readers without recall importing codegraph.

Files: coreutils/pkg/recall/{context.go,rings.go} (new), recall.go (Query/Reader extensions), cmd.go (ring path + the `context` cobra subcommand), testdata/kb-context-envelope.json, tests.
Gate: go test ./pkg/recall/... ./pkg/kb/...; script/memory-eval (umbrella) before/after with thresholds unchanged (>=93% hit@3, >=80% abstention, <=117 tok/query) — on regression raise the evidence bar, never touch the ranker; a budget of 700 over three rings never emits more than 700 tokens; output validates field-for-field against the golden; a missing ring dir exits 1 naming the path.
Traps: pkg/kb stays a leaf; recall must not import codegraph/gfy; tests set BASHY_KB_DIR / BASHY_HOME / BASHY_SKILLS_DIR / YCODE_DATA_DIR to scratch.
Depends on: C1 (agent ring + form), U1 (envelope golden, landed in dhnt 62d3e3f).
