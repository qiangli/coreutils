---
id: 95fcce29a7cc
kind: task
title: S88 pax shared blocker cluster causal closure
seq: 15
status: wontfix
priority: p0
created: 2026-08-31T23:31:56.759938Z
assignee: s88_pax_cluster
closed: 2026-09-30T15:33:56.72117Z
---

Investigate POSIX Profile D pax shared FAIL seats 155,168,185,207,225,245,246,247 plus candidate pax:46 from public-safe metadata and source/history. Identify and implement only a concrete standards-aligned smallest fix with focused native tests; otherwise record the exact redacted evidence tuple and a public reducer plan. No licensed suite or journal bytes.

Public/source review at `origin/main` `2c3fe799` found no lawful TP-to-capability
mapping. The current tree already contains the complete recent pax correction
series through `95698670` and `85578904`; `go test ./cmds/pax` passes. The
public nine-capability POSIX probe is green, while the current matched C and D
replays still report `pax:46=FAIL`. The eight shared FAIL identities occur
under distinct provider implementations, so they remain open rather than
exonerated, but equal numeric result codes do not identify a product cause.

Before a product patch, collect a fixed-vocabulary tuple for each identity:
phase (`parse`, `write`, `list`, `read`, `copy`, `cleanup`, or `unknown`);
feature-category counts (`format`, `extended_header`, `listopt`,
`append_update`, `blocksize`, `selection`, `substitution`, `traversal`,
`preserve`, `link`, `locale`, `path`, `diagnostic_exit`); provider exit status;
stdout/stderr byte and line counts plus SHA-256; and counts for the generic
tokens `archive`, `error`, `option`, `path`, `directory`, `permission`, `open`,
`read`, `write`, `expected`, and `actual`. Do not emit strings, operands, or
journal text.

Use that tuple to select one independently authored reducer from the following
public matrix: archive-format/header round trip; mode/option legality and exit
status; append/update and physical blocking; list/listopt rendering; pattern,
`-n`/`-c`/`-d`, and substitution selection; `-H`/`-L`/`-X` traversal;
preservation/hard-link behavior; locale translation; or pathname-boundary
handling. A reducer must first reproduce the named category outside the suite;
only then should the smallest standards-aligned product change be proposed and
the exact seat replayed uncapped.

## Review 2026-09-30 (steward)

- Status: unknown at today's pins; no cmds/pax commit since 2026-09-01. The eight shared FAIL seats and pax:46 were measured at Sprint 85/88 pins.
- Outdated: "origin/main 2c3fe799", "current matched C and D replays" (Sprint 88), and the claim "`go test ./cmds/pax` passes" - true on darwin only (see 8335807f5495); assignee seat s88_pax_cluster is gone.
- Next step: first land 8335807f5495 (Linux pax defect). Then take pax:* from the fresh baseline full arm + GNU control at the frozen candidate (Sprint 110 f093f2d6bba7); for each still-FAIL identity extract the fixed-vocabulary tuple above and pick one public reducer.
- Acceptance: each pax identity PASS, or shared-with-control with evidence at the candidate digest, or a reducer-reproduced cause + focused test + exact replay PASS.
- Depends on: 8335807f5495; Sprint 110 baseline arm. Overlaps d0114f299697 (pax:46) - one owner for pax:46.
