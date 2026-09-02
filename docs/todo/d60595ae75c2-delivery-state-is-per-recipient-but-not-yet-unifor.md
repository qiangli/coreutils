---
id: d60595ae75c2
kind: task
title: delivery state is per-recipient but not yet uniform across every source
seq: 34
status: todo
priority: p1
created: 2026-09-02T18:50:58.857694Z
---

Carried out of dhnt sprint 99 (story 8874d3675153) partially implemented.

DONE. A meet tell reports per recipient: the resolved addressee is rendered to the sender ("→ conductor:99 (currently trestle)") followed by a state per recipient (unverified / queued / read / failed), and mb send reports unverified rather than claiming delivery. pkg/meet/meet.go writeTellReceipts + boardDeliveryState.

NOT DONE. The same shape does not cover bus notifications or role addresses, so a sender learns different amounts depending on which surface it used. The acceptance was "uniform across mb, rooms, bus and role addresses".

THE RULE THE VOCABULARY MUST FOLLOW, and it is now evidence rather than theory: transport acceptance is NOT semantic handling. A message durable on a board is queued, never delivered. Measured 2026-09-02 (dhnt/docs/agent-wakeup-vendor-surfaces.md section 5): three cross-session messages were accepted by the transport, held for a human approval that never came, and expired — while a status field read as though a turn had started. Anything that reports a success state must be able to distinguish those.

Do not invent a second ladder: dhnt story 70211f1503a2 owns the vocabulary (queued -> claimed -> scheduled -> transport-accepted -> turn-started -> turn-ended -> handled, plus unreachable/failed/timeout). Take it from there.
