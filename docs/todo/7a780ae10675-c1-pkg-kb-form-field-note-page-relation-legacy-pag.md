---
id: 7a780ae10675
kind: task
title: 'C1 pkg/kb form: field (note|page|relation; legacy=page) + agent ring under agent-data via pkg/scope; --ring/--form flags; owner-only read'
seq: 117
status: todo
priority: p0
created: 2026-09-13T01:31:31.604715Z
sprint: 163
---

Goal: pkg/kb gains the two record-level facets: form and ring.

form
- New frontmatter field form: note | page | relation (relation is stored in graph.jsonl, see C5, but the reader vocabulary includes it) — code is a view and is never a stored record. Legacy pages without form: read as page; the writer always emits form:. Type (lesson|gotcha|runbook|decision|fact) stays the WHY of a page/note and is unchanged. note: description optional (ranked when present), body free.
- kb list/search/show accept --form; kb add/note add set it.

ring
- pkg/scope resolution gains the agent ring: an owner-only store under the agent-data dir bashy already relocates per identity (coreutils/pkg/agentlaunch YcodeDataDir / chat agent-data) — path <agent-data>/kb/. Existing repo (docs/kb/) and host (~/.bashy/kb) resolution unchanged; precedence agent > repo > host for reads only when --rings names them; writes go to exactly one ring (--ring, default: repo if in a git repo else host; agent only when explicitly named).
- The agent ring is read only by its owning principal (ToolID/agent name match); other principals see nothing from it. chat clone copies it because it lives under the copied dir (coreutils/pkg/chat/clone.go) — verify, do not re-implement.
- One principal, one attribution: Source.Tool still stamps the writer; the ring is store precedence, NOT an axis on the record (docs/agent-character-role-and-episodes.md I1 as amended by U1).

Files: coreutils/pkg/kb/page.go (Form field, defaults, ValidForm), store.go (ring root), kb.go (flags), coreutils/pkg/scope/scope.go (agent ring kind), tests in kb_test.go / scope_test.go.
Gate: go test ./pkg/kb/... ./pkg/scope/...; existing contract tests unchanged (TestSupersedeAndValidateLadder, TestConcurrentWrites, TestSourcesAndTransferAreReadOnly, TestRepoStoreDoesNotNestGit, TestOrdinaryKBWritesPublishNothing); new: legacy page reads as form page; agent ring resolves under agent-data and is invisible to another principal; a write names exactly one ring. Tests must set BASHY_KB_DIR / BASHY_HOME / the agent-data env to scratch — never the operator's stores.
Traps: pkg/kb is an import leaf (may import scope, bus, git only). Do not add a related: field. Do not touch Search ranking.
Depends on: U1.
