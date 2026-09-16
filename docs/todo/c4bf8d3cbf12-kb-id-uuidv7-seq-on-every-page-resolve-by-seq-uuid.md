---
id: c4bf8d3cbf12
kind: task
title: 'kb: id (UUIDv7) + seq on every page; resolve by seq/uuid/slug; doctor --fix stamps'
seq: 149
status: todo
priority: p0
created: 2026-09-16T18:56:50.607253Z
sprint: 202
---

Sprint #202 story 2 (coreutils, the real work). Depends on story 1 (ref.ShapeOf / SplitScope / Node UID+Seq). A runbook is a kb page (type: runbook), so this is what makes kb:22, kb:<uuid-prefix> and kb:release-cycle all open the same runbook.

pkg/kb:
- Page gains ID string (yaml id, UUIDv7 via uuid.NewV7 — google/uuid is already a direct dep) and Seq int (yaml seq). Slug stays the filename stem.
- Store.Write mints ID when empty and assigns Seq when 0 as MaxSeq+1 over the ring (todo's mechanics, reused). NEVER mint on read — a repo-ring read must not produce a commit. Journal record gains id.
- LoadBySeq(n), LoadByUID(prefix): List + filter; a prefix matching >1 page is a named ambiguity error, never ErrNotFound.
- RegisterRefs gains scopeLookup; resolveKB: scope segment -> that repo's ring via scopeLookup (unknown scope = ErrNotFound naming it), else today's ring order (cwd repo ring, then host). Dispatch on ShapeOf(local). Node emits Ref = kb:<slug> (seq is input-only, never emitted — meet's room-number rule), plus UID and Seq.
- kb show accepts the same three spellings (share the resolver path).
- kb add / --slug: refuse a slug whose ShapeOf != ShapeSlug, with a message saying why.
- kb list and index.md print a #seq column; --json gains id, seq, ref.
- kb doctor: report pages missing id/seq and duplicate seq within a ring (merge collision); --fix stamps missing ids, backfills seq in created order (EnsureSeq-style) and renumbers the later-created duplicate. Idempotent: second --fix is a no-op.
- Tests hermetic under BASHY_KB_DIR; TestKBIsALeaf stays green; supersede still keeps the old slug resolving with Successor (D6 unchanged).

Out of scope: slug rename/redirect; write-on-read backfill; any new ref kind.
