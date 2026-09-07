---
id: 86a6cc12c1e4
kind: task
title: 'The meet e2e suite has one pre-existing red case: bounded Chat progress never renders'
seq: 95
status: todo
priority: p2
created: 2026-09-07T19:02:55.442436Z
sprint: 135
---

FOUND while gating story e9f5d325eded on 2026-09-07, and it is NOT a regression from that work.

THE FAILURE. pkg/meet/web/e2e/meet-room.spec.ts, "a long Chat reply shows bounded real output and cumulative progress": it sends "show sampled progress" to a Chat and waits for the [data-live-progress] card. The locator never becomes visible and the case times out.

THE A/B, which is the part that matters. The e9f5d325eded change touched exactly this file and the composer it drives, so "my change broke it" was the obvious reading and it is WRONG. Stashing the whole working tree (src/ and e2e/) back to unmodified main and re-running the same single case reproduces the same red. The case fails on both sides; the removal of the send hold is not implicated.

WHY IT MATTERS ANYWAY. It means the meet e2e suite cannot currently be reported green, and this sprint's acceptance says every story lands with a runnable gate. Everything else passes — 26 of 26 with this one case excluded — but a suite with a permanently excluded case is one step from a suite nobody runs. docs/fleet-evidence-invariant.md is the rule being protected: no success state may be reached by the ABSENCE of evidence, and quietly grepping a red case out of the run is exactly that.

OPEN, and to be settled by whoever takes this. Which of these it is has NOT been determined: (a) the progress card genuinely regressed at some earlier commit and the test is correct; (b) the test asserts a guarantee the streaming path never made, in which case it belongs with docs' record of CI flakes that asserted false guarantees; (c) it is timing-sensitive against the fixture agent and needs a wait on a real signal rather than visibility. Bisect before choosing — the sampled-progress path was last touched by 9e064495 'meet: stream bounded Chat progress'.

DO NOT fix it by widening the timeout without first establishing which of the three it is. A longer wait on a card that never renders buys nothing, and a longer wait on a race hides it.

GATE. `npx playwright test --grep "a long Chat reply shows bounded"` from pkg/meet/web goes green, and the FULL suite runs with no --grep-invert.
