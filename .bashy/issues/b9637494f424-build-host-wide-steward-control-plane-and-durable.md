---
id: b9637494f424
kind: feature
title: Build host-wide steward control plane and durable board
status: triaged
stage: code
priority: p0
refs:
    - bashy
labels:
    - steward
    - continuity
    - host-control
reporter: codex
created: 2026-07-13T20:15:32.506404Z
weave: 88
---

GOAL
Implement the core host/user-scoped steward subsystem agreed in the 2026-07-13 three-party meeting. This is authority and continuity, not project working-tree handoff.

DESIGN CONTRACT
- Exactly one steward seat per host/user, stored host-wide and independent of cwd/repository.
- Reuse coordination ideas: heartbeat lease, atomic acquisition, monotonically increasing fencing epoch, and human-authorized takeover. Never require incumbent communication or a handoff note.
- One append-only evidence-carrying journal is authoritative. Board, status, log, conversation, history, and checkpoints are read-only projections.
- Authority classes: effect/evidence events; explicit decision records; optional non-authoritative hash-linked transcript artifacts.
- Missing evidence yields unknown/degraded, never success.
- Generic pkg/handoff behavior remains task/artifact scoped. Do not restore or mutate a repository when claiming the steward seat.

COREUTILS SCOPE
Create a cohesive pkg/steward implementation and Cobra command constructor suitable for bashy mounting. Prefer stdlib and existing principal/coord patterns. Store under BASHY_STEWARD_DIR or ~/.bashy/steward.

REQUIRED CLI
- steward status: seat holder/epoch/liveness and concise current board
- steward board: list and show workstreams
- steward log: chronological events, filters and --json; support follow only if cleanly testable
- steward conversation: decision/conversation events
- steward history: state/checkpoint history
- steward checkpoint: materialize a verified checkpoint from the journal
- steward reconcile: explicit reconciliation event/report with unknown/degraded support
- steward claim or take: acquire vacant/expired seat atomically
- steward takeover: explicit human-authorized epoch-bumping recovery, fencing the prior holder
- steward release or transfer preparation, without repository work capture

DATA/FAILURE REQUIREMENTS
- Schema-version every artifact.
- Atomic durable writes and serialized read/decide/write on Unix; preserve an honest Windows limitation if LockFileEx is not available.
- Journal entries include id, timestamp, actor, steward epoch, kind, workstream/ref fields, summary, rationale, evidence references and optional artifact digest.
- Checkpoints include journal watermark/digest and must be reproducible projections, never a competing writable truth.
- A stale heartbeat proves only a liveness lapse.
- Old-epoch mutation is rejected.
- Corrupt/truncated final journal data does not hide prior valid history.
- Human-readable output plus stable --json wherever appropriate.

TESTS
Cover singleton acquisition, heartbeat/staleness, human takeover, fencing, crash recovery without handoff notes, append-only replay, corrupt-tail recovery, checkpoint reproducibility, view derivation, unknown/degraded preservation, and a transcript-deletion recovery test proving transcripts are optional.

DOCS
Package docs and a focused design/use document. Reference the meeting decisions without depending on private umbrella-only content.

BOUNDARIES
Only edit this isolated coreutils workspace. Do not edit bashy, umbrella, or other sibling repos. Commit all work. Run focused tests plus the full hermetic coreutils test scope. If scope is too large, deliver a coherent vertical MVP with explicit remaining gaps rather than shallow stubs.
