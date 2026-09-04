---
id: af802f2fafed
kind: task
title: 'GATED: pkg reducer core — spill store, elision marker, and the bashy out verb'
seq: 42
status: todo
priority: p2
created: 2026-09-04T09:52:41.171005Z
sprint: 123
---

CLOSED until Yoke gates 1 (#100 POSIX) and 2 (#98 Bash++) are met. Stage A1 from the design: content-addressed spill written BEFORE any output is emitted, 0600, through the existing command/session/run artifact path (no seventh store). Marker carries omitted count + digest + a RUNNABLE recovery command + the prevention hint. Reuse pkg/admission (UTF8Prefix, Overflow manifest, digests) but note UTF8Prefix REFUSES invalid UTF-8 and command output often is not — needs an explicit binary path: detect, do not repair.
