---
id: 95fcce29a7cc
kind: task
title: S88 pax shared blocker cluster causal closure
seq: 15
status: todo
priority: p0
created: 2026-08-31T23:31:56.759938Z
assignee: s88_pax_cluster
sprint: 100
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
