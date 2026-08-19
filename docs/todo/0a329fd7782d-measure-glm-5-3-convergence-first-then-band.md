---
id: 0a329fd7782d
kind: task
title: 'Measure glm-5.3: convergence first, then band'
seq: 7
status: todo
priority: p1
created: 2026-08-19T16:54:40.745385Z
---

glm-5.3 shipped as band 2 / band_source: declared — a PRIOR carried from 5.2, not a measurement. Every surface renders it L2~ until someone runs the ladder. Agent: ycode-glm-5.3 (Kenji). Live-verified reachable 2026-08-19 (agents verify --live: launched headless and answered) — that is reachability, NOT capability.

MEASURE CONVERGENCE FIRST, because it is what pegged 5.2 and it is the coder/lead line:
5.2 is L2 despite genuinely good coding and finding (3/3 gate PASS; it found two real bugs an L4 missed) purely because IT NEVER STOPS — it hit the iteration cap in 3/3 exams (25/80/60 turns) and produced no synthesized report in any of them. Its findings survived only because a human read its stderr. If 5.3 converges, it is a different band; if it does not, it inherits 5.2's routing rule verbatim (route work TO it with a gate and a bounded scope; never seat it as conductor or steward).

CHEAPEST SIGNAL: the loop metric from docs/band-ladder.md — total tool calls / distinct tool calls. 5.2-class failure looks like gemini3.1's 9.4x; a converging L3 looks like deepseek-v4-pro's 1.2x.

THEN: set band_source: measured with the evidence inline in pkg/fleet/baseline/models/glm-5.3.yaml. Do NOT edit glm-5.2.yaml — it is the record of what 5.2 did.

ALSO CONFIRM (both inherited from 5.2's file and unverified on 5.3): the 1302-vs-1308 quota split (rate limit clears in seconds; usage limit is gone for hours) and the ~8 concurrent-request ceiling on the Pro plan.

BLOCKS: moving the four ycode cascades' base: from ycode-glm-5.2 to ycode-glm-5.3, and retiring Xavier. Both wait on this number.
