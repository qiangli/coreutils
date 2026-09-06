---
id: af802f2fafed
kind: task
title: 'GATED: pkg reducer core — spill store, elision marker, and the bashy out verb'
seq: 42
status: todo
priority: p1
created: 2026-09-04T09:52:41.171005Z
sprint: 123
---

OPEN. The operator opened Sprint #123 on 2026-09-04, superseding the original
gate on Yoke gates 1 (#100 POSIX) and 2 (#98 Bash++) — gate 2 is done, gate 1's
seat is unassigned, and this body is edited in its own implementation commit per
the umbrella story-body obligation (`docs/command-output-reduction-design.md`).

Stage A1 from the design: content-addressed spill written BEFORE any output is
emitted, 0600, through the existing command/session/run artifact path (no seventh
store). Marker carries omitted count + digest + a RUNNABLE recovery command + the
prevention hint. Reuse pkg/admission (UTF8Prefix, Overflow manifest, digests) but
note UTF8Prefix REFUSES invalid UTF-8 and command output often is not — needs an
explicit binary path: detect, do not repair.

Landed (this commit): `pkg/reduce` — the store-neutral A1 core.
- `Store` (store.go): content-addressed spill over a host-supplied artifact root
  (no seventh store). Blobs named by full sha256, written 0600 via temp+rename
  (lock-free, atomic; readers never see a partial blob). Idempotent `Put`;
  `Get`/`resolve` accept a bare hex prefix, a full digest, or a `sha256:` handle,
  and report an ambiguous prefix so the caller lengthens it.
- `Reduce` (reduce.go): enforces the byte ceiling (default 40 KiB), spills the
  complete (optionally redacted) bytes BEFORE emitting, and places the marker at
  the elision site — `[bashy: N lines / X KiB elided · sha256:… · full: bashy out
  <handle> · keep: BASHY_OUTPUT_REDUCE=off]`. Redaction precedes spill (§2.7) via
  an injected `Redactor` (secrets.Redactor satisfies it). Binary output (invalid
  UTF-8) is detected, not repaired: represented as a header + handle, never
  inlined. Deterministic: identical input reduces to byte-identical output.
- `Recover` / `NewOutCmd` (out.go): the recovery engine/constructor for an
  eventual `bashy out <id>` command. The actual command tree mount and artifact
  resolver belong to the execution-seam stories; this commit alone does not
  make `bashy out` reachable from bashy.

Still owed by the wider phase (other stories / bashy-side): wiring the seams
(chat.Invoke, WireExec shell path, `bashy run --capture`), the `--no-elide`
override helper in pkg/weavecli, and A2/A3+ pipeline stages.
