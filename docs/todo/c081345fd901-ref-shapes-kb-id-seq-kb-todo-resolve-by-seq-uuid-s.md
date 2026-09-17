---
id: c081345fd901
kind: task
title: 'ref shapes + kb id/seq + kb/todo resolve by seq/uuid/slug[/scope] (S1, merges #149)'
seq: 148
status: doing
priority: p0
created: 2026-09-16T18:56:38.594073Z
sprint: 202
---

Sprint #202 S1 (coreutils) — the whole coreutils half; was stories #148 + #149, merged (same owner, same pin, 149 depended on 148). Plan: docs/sprint-202-master-execution-plan.md in the umbrella.

Vocabulary: an ENTITY is anything with a ref (one of the 15 ref.Kinds()); ref = address, kind = category, record = stored form. Three handles per entity: seq (humans, #22, unique within its scope), uuid (agents/cross-host, universal, the identity), slug (web/URL, unique within its scope). Scope = the store the entity lives in, nameable as a leading path segment, elidable in context. seq is accepted as input, never emitted as a ref.

pkg/ref (stdlib only, leaf pin stays):
- ShapeOf(local) Shape: len >= 12 hex or dashed uuid form -> ShapeUID; all-decimal without leading zero -> ShapeSeq; >= 8 hex -> ShapeUID (unique prefix); anything else -> ShapeSlug. (The >= 12 rule keeps an all-digit 12-hex todo id reachable; UUIDv7 prefixes start with 0.)
- SplitScope(id) (scope, local): split on the FIRST "/"; scope empty when absent. cleanID strips "#" on the local part only.
- Node gains UID string (json uid) and Seq int64 (json seq). ID/Ref semantics unchanged.
- Package doc: one paragraph defining entity + the three handles.
- Table test incl. 0192f3a4 -> uid, 12345678 -> seq, deadbeef -> uid, release-cycle -> slug, cafe -> slug, "#3" -> seq, 123456789012 -> uid.

pkg/kb:
- Page gains ID (yaml id, UUIDv7 via uuid.NewV7; google/uuid already a dep) and Seq (yaml seq). Slug stays the filename stem.
- Store.Write mints ID when empty and assigns Seq when 0 as MaxSeq+1 over the ring. NEVER mint on read — a repo-ring read must not produce a commit (todo's EnsureSeq-on-list is the recorded asymmetry, not copied). Journal record gains id.
- RegisterRefs gains scopeLookup func(name) (root, err) (bashy wires it; nil = no scopes). resolveKB: scope segment -> that repo's ring (unknown scope = error naming it), else today's ring order. Dispatch on ShapeOf(local): seq -> by Seq; uid -> exact-then-unique-prefix; slug -> Load. Ambiguity (prefix or duplicate seq) = a named error listing the candidates, never ErrNotFound. Node emits Ref = kb:<slug> plus UID and Seq.
- kb show accepts the same three spellings (same resolver path).
- kb add / --slug: refuse a slug whose ShapeOf != ShapeSlug, saying why.
- kb list and index.md print #seq; --json gains id, seq, ref.
- kb doctor REPORTS pages missing id/seq and duplicate seq within a ring. NO --fix (doctor flags, never fixes — pinned by TestDoctorLeavesBodyByteIdentical). Backfill = an explicit write per page (kb update <slug> goes through Write); a duplicate seq is reported and kb:<seq> is ambiguous while slug + uuid keep resolving — no renumbering.
- Tests hermetic under BASHY_KB_DIR; TestKBIsALeaf green; supersede keeps the old slug resolving with Successor (D6 unchanged).

pkg/todo:
- RegisterRefs gains the same scopeLookup. resolveTodo: scope segment -> that store (repo root via scopeLookup, "user" -> personal); no scope -> today's cwd store. Dispatch on ShapeOf(local): seq -> by Seq; uid -> today's exact-then-prefix; slug -> filename slug (<id>-<slug>.md), ambiguous = named error. Node fills UID (the 12-hex id) and Seq. Hex behaviour unchanged.
- Hermetic tests under BASHY_HOME (never the operator's real store).

Out of scope: widening the 12-hex id; slug uniqueness at add for todo; emitting scoped refs; slug rename; any new ref kind.
