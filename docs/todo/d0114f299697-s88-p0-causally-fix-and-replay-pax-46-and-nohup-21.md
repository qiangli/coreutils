---
id: d0114f299697
kind: task
title: S88 P0 — causally fix and replay pax:46 and nohup:21
seq: 13
status: todo
priority: p0
created: 2026-08-31T23:29:09.397087Z
assignee: codex-s88-hash9
sprint: 100
---

Frozen Coreutils c747cab uncapped Profile D replay still reports pax:46=FAIL and nohup:21=FAIL. Reduce from tracked public-safe evidence and redacted category/count/hash tuples only; do not inspect or export licensed journals. Implement only concrete standards-aligned product causes, with smallest focused native tests on Dragon. Aurelia owns Novi exact replay, review, merge, push, pins, and cleanup.

## Review 2026-09-30 (steward)

- Status: unknown at today's pins. Measured at frozen coreutils c747cab (Sprint 85/88). Since then nohup changed only in its test deadlines (127ef7a1), and pax not at all; launcher/exec-budget commits (753cdd0f, b00181f5) touch nohup's exec path.
- Outdated: frozen c747cab, the named replay host, assignee seats codex-s88-hash9 / aurelia-s88.
- Next step: take pax:46 and nohup:21 from the fresh baseline full arm at the frozen candidate (Sprint 110 f093f2d6bba7). pax:46 belongs to 95fcce29a7cc (dedupe: keep pax:46 there and narrow this story to nohup:21). nohup:21 was "control-unstable" in Sprint 85 - compare with the GNU control before any product change.
- Acceptance: nohup:21 PASS, or shared/unstable-in-control disposition with evidence at the candidate digest, or a mapped cause + focused test + exact replay PASS.
- Depends on: Sprint 110 baseline arm.
