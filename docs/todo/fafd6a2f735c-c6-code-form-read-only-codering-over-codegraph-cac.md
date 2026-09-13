---
id: fafd6a2f735c
kind: task
title: 'C6 code form: read-only CodeRing over codegraph cache + repomap within budget; codegraph deterministic node ids + DIRECTED edges (E7)'
seq: 122
status: todo
priority: p2
created: 2026-09-13T01:31:31.748194Z
sprint: 163
---

Goal: the code form — code intelligence for coding-agent context engineering — as a read-only VIEW over the existing engines, served through the same front door. Never stored as kb records, never validated, never exported.

- CodeRing reader in pkg/kb over pkg/codegraph's cache (.agents/bashy/graph.json; build-or-load exactly as bashy graph does, mtime-stale) plus pkg/repomap for the budgeted map. kb search --form code and kb context --forms code return symbols / files / impact blocks with ref code:<id> within the per-ring budget (default depth 1, limit 40 — the measured token trap: depth 2 on the undirected graph cost ~7k tokens).
- Engine fixes decided but not built (E7): deterministic node ids (gfy ids differ run-to-run) and DIRECTED edges at extraction (caller != callee) so relation (C5) can join to code nodes and impact means reverse dependency, not coupling.
- "Files that matter for this task": kb context --files boosts chat files (repomap ChatContextFiles) and --for text drives RelevanceQuery; no new ranker beyond what repomap already does.
- bashy graph / bashy ast verbs unchanged on the surface; no yc resurrection; no bonsai mirror work (MirrorTo stays unused).

Files: coreutils/pkg/kb/codering.go (new), coreutils/pkg/codegraph/{codegraph.go,mirror.go} (ids, directed), pkg/repomap (no change expected), tests.
Gate: go test ./pkg/kb/... ./pkg/codegraph/...; two builds of the coreutils tree yield identical node ids; neighbors --relation calls distinguishes callers from callees; kb context --forms code --budget 700 on this repo emits <= 700 tokens.
Depends on: C3.
