---
id: c081345fd901
kind: task
title: 'ref: ShapeOf/SplitScope + Node UID/Seq; todo ref resolves seq/slug/scope'
seq: 148
status: todo
priority: p0
created: 2026-09-16T18:56:38.594073Z
sprint: 202
---

Sprint #202 story 1 (coreutils). Vocabulary: an ENTITY is anything with a ref (one of the 15 ref.Kinds()); ref = address, kind = category, record = stored form. Three handles per entity: seq (humans, #22, unique within its scope), uuid (agents/cross-host, universal), slug (web/URL, unique within its scope). Scope = the store the entity lives in, nameable as a leading path segment and elidable in context.

pkg/ref (stdlib only, leaf pin stays):
- ShapeOf(local) Shape: decimal without leading zero -> ShapeSeq; >= 8 hex, optionally dashed uuid form -> ShapeUID (uuid or unique prefix); anything else -> ShapeSlug.
- SplitScope(id) (scope, local string): split on the FIRST "/"; scope empty when absent.
- cleanID strips "#" on the local part only.
- Node gains UID string (json uid) and Seq int64 (json seq). ID/Ref semantics unchanged.
- Package doc: one paragraph defining entity + the three handles.
- Table test: 0192f3a4 -> uuid, 12345678 -> seq, deadbeef -> uuid, release-cycle -> slug, cafe -> slug, "#3" -> seq.

pkg/todo:
- RegisterRefs gains scopeLookup func(name string) (root string, err error) (bashy wires it; nil = no scopes).
- resolveTodo: scope segment -> that store (repo root via scopeLookup, "user" -> personal); no scope -> today's cwd store selection. Then dispatch on ShapeOf(local): seq -> by Seq (what `todo show 3` means today); uuid -> today's exact-then-unique-prefix; slug -> filename slug (<id>-<slug>.md), ambiguous = named error, never ErrNotFound.
- Node fills UID (the 12-hex id) and Seq. Hex-id behaviour unchanged.
- Hermetic tests under BASHY_HOME (never the operator's real store).

Out of scope: widening the 12-hex id; enforcing slug uniqueness at add (doctor reports only); emitting scoped refs.
