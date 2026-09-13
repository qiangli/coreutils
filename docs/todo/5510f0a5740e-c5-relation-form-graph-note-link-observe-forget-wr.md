---
id: 5510f0a5740e
kind: task
title: 'C5 relation form: graph note|link|observe|forget write <ring>/kb/graph.jsonl (committed for repo ring), deterministic sha1(kind\0name)[:16] ids, core relation vocabulary; RelationRing reader'
seq: 121
status: todo
priority: p1
created: 2026-09-13T01:31:31.720373Z
sprint: 163
---

Goal: the relation form — entity/relationship claims that skills compose over — moves under the kb front door without a new noun.

- graph note | link | observe | forget keep their names and flags but write to <ring>/kb/graph.jsonl (repo ring: committed under docs/kb/; host ring: ~/.bashy/kb/) — closes the documented contradiction that contrib.jsonl lives under gitignored .agents/. Append-only JSONL, O_APPEND, soft-delete replay exactly as coreutils/cmds/graph/contrib.go does today.
- Deterministic entity ids: sha1(kind\0name)[:16] (the kg pattern) so a relation can point at a code node, a page slug, a todo id or a principal with one id shape. Target/Dst keep the human string alongside the id.
- Core relation vocabulary: about, supersedes, depends-on, calls, tested-by, decided-in, observed. Open vocabulary is accepted but flagged by kb doctor (C2).
- kb search --form relation and kb context --forms relation read graph.jsonl through a RelationRing reader with the same envelope; --federate keeps working for old contrib.jsonl locations (read-only).
- Craft folds stay in craft (executable rendering; Coordinate == ring+scope); facts never become relations.

Files: coreutils/cmds/graph/{contrib.go,contrib_verbs.go}, coreutils/pkg/kb/relation.go (new reader), tests (replay/forget semantics identical to contrib_test).
Gate: go test ./cmds/graph/... ./pkg/kb/...; ids stable across two runs; forget still hides; a relation with an open-vocab type shows in doctor output; nothing is written to .agents/bashy/graph/ any more.
Depends on: C1, C2 (doctor).
