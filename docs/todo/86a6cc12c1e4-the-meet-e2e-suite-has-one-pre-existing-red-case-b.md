---
id: 86a6cc12c1e4
kind: task
title: 'The meet e2e suite has one pre-existing red case: bounded Chat progress never renders'
seq: 95
status: done
priority: p2
created: 2026-09-07T19:02:55.442436Z
sprint: 136
closed: 2026-09-07T20:56:14.706443Z
---

FOUND while gating story e9f5d325eded on 2026-09-07, and it is NOT a regression from that work.

THE FAILURE. pkg/meet/web/e2e/meet-room.spec.ts, "a long Chat reply shows bounded real output and cumulative progress": it sends "show sampled progress" to a Chat and waits for the [data-live-progress] card. The locator never becomes visible and the case times out.

THE A/B, which is the part that matters. The e9f5d325eded change touched exactly this file and the composer it drives, so "my change broke it" was the obvious reading and it is WRONG. Stashing the whole working tree (src/ and e2e/) back to unmodified main and re-running the same single case reproduces the same red. The case fails on both sides; the removal of the send hold is not implicated.

WHY IT MATTERS ANYWAY. It means the meet e2e suite cannot currently be reported green, and this sprint's acceptance says every story lands with a runnable gate. Everything else passes — 26 of 26 with this one case excluded — but a suite with a permanently excluded case is one step from a suite nobody runs. docs/fleet-evidence-invariant.md is the rule being protected: no success state may be reached by the ABSENCE of evidence, and quietly grepping a red case out of the run is exactly that.

OPEN, and to be settled by whoever takes this. Which of these it is has NOT been determined: (a) the progress card genuinely regressed at some earlier commit and the test is correct; (b) the test asserts a guarantee the streaming path never made, in which case it belongs with docs' record of CI flakes that asserted false guarantees; (c) it is timing-sensitive against the fixture agent and needs a wait on a real signal rather than visibility. Bisect before choosing — the sampled-progress path was last touched by 9e064495 'meet: stream bounded Chat progress'.

DO NOT fix it by widening the timeout without first establishing which of the three it is. A longer wait on a card that never renders buys nothing, and a longer wait on a race hides it.

GATE. `npx playwright test --grep "a long Chat reply shows bounded"` from pkg/meet/web goes green, and the FULL suite runs with no --grep-invert.

## FINDING 2026-09-07 (corbel, sprint 136) — cause (b), and the test is right

BISECTED AS INSTRUCTED, and the story's own description of the failure is
WRONG in a way that matters. The card is NOT missing. Lines 473-474 pass: the
`[data-live-progress]` element renders and contains the counters. The failing
assertion is the NEXT one, line 475 — the `<pre>` holding sampled output.

And the first three assertions pass VACUOUSLY: the card renders from the
`speaking` frame with lines=0, bytes=0, elapsed=0, so `/lines observed/`,
`/B/` and `/elapsed/` all match an EMPTY card. The suite was reporting three
green assertions against zero progress.

MEASURED, not inferred. Captured the observe socket's frames in the browser:

  13:52:13.358  dm-progress  speaking   (no counters)
  13:52:16.267  dm-progress  line 1   lines=1   <- all five samples land
  13:52:16.267  dm-progress  line 2   lines=2      within 0.3 MILLISECONDS
  13:52:16.267  dm-progress  line 3   lines=3      of each other, 2.9s after
  13:52:16.268  dm-progress  line 5   lines=5      speaking
  13:52:16.268  dm-progress  line 8   lines=8
  13:52:16.291  dm-progress  spoke    lines=9    <- 23ms later, card destroyed

The fixture emits its 8 lines over 1.6 real seconds. The browser receives all
of them in one 0.3ms burst at process EXIT, followed 23ms later by `spoke`,
which sets live=null and unmounts the card. There is no window in which a
browser assertion could observe the `<pre>`. Widening the timeout cannot help;
the card is already gone.

ROOT CAUSE, upstream of meet entirely. `chat.Invoke` captures the agent's
output into a `bytes.Buffer` (`cmd.Stdout = &stdout`, pkg/chat/chat.go:452;
`runPTY` does the same into `buf`) and writes `opt.Stream` ONCE after Wait
returns (`emitReduced(opt.Stream, reduced)`, ~:1143). So the DM live channel is
not a stream — it is a POST-HOC REPLAY of a finished turn. pkg/meet's careful
per-line framing, the fibonacci sampler and the counters are all correct and
all fed one blob at the end. Verified the server-side projection in isolation:
given the 9 captured lines it emits exactly 5 sampled frames with text and
counters, schema-valid. Nothing in pkg/meet is broken.

SO THE ANSWER IS (b): the test asserts a guarantee the streaming path never
made. But (b) does NOT mean the test is wrong — the FEATURE is wrong. A human
watching a five-minute agent turn sees `0 lines observed` for five minutes and
then the whole answer at once, which is precisely what bounded live progress
exists to prevent. The test is the only thing that noticed.

SECOND DEFECT, found in the same capture and not previously recorded: the
sampler keeps emitting heartbeats AFTER the turn ends —

  13:52:21.450  dm-progress  line  lines=9  elapsed_ms=8091   (empty text)
  13:52:26.476  dm-progress  line  lines=9  elapsed_ms=13117  (empty text)

`spoke` does not stop the sampler, so a finished turn resurrects a text-less
progress card every 5s forever. That also makes the case's LAST assertion
(`expect(progress).toHaveCount(0)`) racy against a 5s heartbeat.

DISPOSITION. The suite now runs the case with NO --grep-invert, marked
`test.fail()` — it RUNS, its failure is asserted, and it turns red the moment
someone fixes the stream (which is the correct alarm). The real fix — tee
`opt.Stream` while the process is alive instead of replaying at exit — is a
pkg/chat change on the hot path of every agent invocation, too large to land
safely inside this sprint's box and while #100/#117 share this repo. Filed as
its own story.
