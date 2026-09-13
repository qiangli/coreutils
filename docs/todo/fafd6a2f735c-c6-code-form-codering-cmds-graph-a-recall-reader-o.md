---
id: fafd6a2f735c
kind: task
title: 'C6 code form: CodeRing (cmds/graph, a recall.Reader) over codegraph + repomap within budget; codegraph DIRECTED edges (E7 — ids already deterministic, 256/438 edges flipped)'
seq: 122
status: assigned
priority: p2
created: 2026-09-13T01:31:31.748194Z
weave: 13
assignee: qiangli
sprint: 163
---

Goal: the code form — code intelligence for coding-agent context engineering — as a read-only VIEW over the existing engines, served through the same front door. Never stored as kb records, never validated, never exported. Design of record: dhnt docs/kb-rings-forms-stages.md §4 (code), §7; plan D1/D3.

RE-SCOPED BY MEASUREMENT (plan D3). Two builds of the same tree were compared (2026-09-12): node ids are ALREADY deterministic (189/189 identical — gfy/pkg/extract.MakeID is a pure string normalisation), so "deterministic node ids" is a ratchet test, not work. What is broken is DIRECTION: the cache says directed:false and 256 of 438 edges flip source/target between two builds (the undirected container keeps _src/_tgt stable and writes the directed attributes in arbitrary order), so `contains` may point symbol->file and `calls` callee->caller on any given run.

Engine half (independent of C1–C3; can start in wave 1):
- pkg/codegraph/codegraph.go: build.BuildFromResult(extraction, true) — gfy builds directed graphs natively; cluster.Cluster already calls ToUndirected() itself, so community detection is unaffected. Add a cache version key so a stale undirected .agents/bashy/graph.json rebuilds instead of loading. Audit the consumers (neighbors, impact, query, subgraphToText) over directed adjacency: impact = reverse dependency (who depends on X), coupling stays available behind an explicit --undirected flag if any consumer relied on symmetry — record it, never silently restore it.
- No gfy change, no gfy pin.

Reader half (after C3 + the engine half):
- CodeRing implements recall.Reader and lives in cmds/graph (beside the engine; NOT in pkg/kb — leaf pin — and NOT in pkg/recall — keeps gfy + tree-sitter out of it). bashy's mount (internal/agentos/kbrecall.go, B1 landing 2) injects it through the Reader-injection point C3 exposes.
- kb search --form code and kb context --forms code return symbols / files / impact blocks with ref code:<id> within the per-ring budget (default depth 1, limit 40 — depth 2 on the undirected graph measured ~7k tokens). Build-or-load exactly as bashy graph does (mtime-stale) plus pkg/repomap for the budgeted map; --files boosts chat files (repomap ChatContextFiles), --for text drives RelevanceQuery; no new ranker.
- bashy graph / bashy ast verbs unchanged on the surface; no yc resurrection; no bonsai mirror work (MirrorTo stays unused).

Files: coreutils/pkg/codegraph/{codegraph.go,mirror.go} (directed flag, cache version, consumer audit), coreutils/cmds/graph/codering.go (new), pkg/repomap (no change expected), tests.
Gate: go test ./pkg/codegraph/... ./cmds/graph/...; two builds of the same tree yield identical node ids AND zero source/target orientation flips; every `contains` edge is file->symbol; neighbors --relation calls distinguishes callers from callees; a cache written before the version key is rebuilt, not loaded; kb context --forms code --budget 700 on this repo emits <= 700 tokens.
Depends on: engine half — nothing; reader half — C3.
