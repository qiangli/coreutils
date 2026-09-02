---
id: 0892cb5be1d3
kind: task
title: 'meet join: an explicit seat-taking verb that must NOT mint liveness'
seq: 33
status: todo
priority: p2
created: 2026-09-02T18:50:58.833585Z
---

Carried out of dhnt sprint 99 (story 49355baaf899) unimplemented. Verified 2026-09-02: `meet --help` lists no join verb.

WHAT IS MISSING. Seating is organizer-push only (`meet invite`), with self-seat available on an open board via OpenTo. There is no way for an agent to take a seat in a room it was told about. Since delivery now keys on the SEAT (pkg/meet Rooms/attendees, consumed by bashy inbox), not holding one means not receiving — so "join the room" is the natural request and there is no verb for it.

THE TRAP, and it is the whole reason this is P2 rather than P1. A join must NOT make the agent look LIVE. Seat and liveness are different facts with different lifetimes: a seat is durable and addressable, a live process is ephemeral and deliverable. A join that minted liveness would let a stale seat claim a running agent forever, which is the same lie as a green owner mark over unread questions. script/e2e-agent-inbox.sh check A6 already guards exactly this and passes today — it must still pass afterwards.

Gate: check B5 in that suite is written and PLANNED; implementing this promotes it.

Spec: dhnt/docs/agent-inbox-unified-delivery.md sections 2 and 7 (P2).
