---
id: 7bd979b80383
kind: task
title: 'C8 kb transfer --from memex <dir>: move the owner''s own memex store into its agent ring as form: note candidates (foreign stores stay pointers-not-copies)'
seq: 124
status: todo
priority: p2
created: 2026-09-13T01:31:31.804595Z
sprint: 163
---

Goal: kb transfer --from memex <dir> moves an agent's OWN ycode memex store into its agent ring as form: note candidates, so memex can be retired from the ycode harness (D1/D4).

- Reads ~/.agents/ycode/memory and <repo>/.agents/ycode/memory (the two locations kb sources already probes as frontmatter-md), maps Memory{Name,Description,Type,Scope,Content,Tags,ContentHash,SupersededBy,ValidUntil} -> Page{form: note, status: candidate, tags + xfer:memex, Supersedes chain preserved, Source.Tool = memex}; skips items with ValidUntil in the past; dedups by ContentHash then NearDuplicate.
- Pointers-not-copies applies to OTHER agents' stores (kb transfer's existing rule, pinned by TestSourcesAndTransferAreReadOnly); the owner's own memex is a MOVE into its own ring, and the source dir is left untouched (report only; the operator deletes).
- kb sources reports the dir as transferred afterwards (counts only, as today).

Files: coreutils/pkg/kb/{transfer.go,sources.go}, tests with a fixture memex dir.
Gate: go test ./pkg/kb/...; TestSourcesAndTransferAreReadOnly still passes for foreign stores; the fixture transfers N notes into a scratch agent ring and a second run transfers 0.
Depends on: C1, C4 (note add path).
