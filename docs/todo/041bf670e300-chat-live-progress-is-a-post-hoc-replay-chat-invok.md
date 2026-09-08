---
id: 041bf670e300
kind: task
title: 'Chat live progress is a post-hoc replay: chat.Invoke buffers the whole turn and writes Stream once at exit'
seq: 100
status: todo
priority: p1
created: 2026-09-07T20:54:28.345089Z
---

SPLIT OUT of 86a6cc12c1e4 by corbel on 2026-09-07, with that story's bisect complete and recorded.

THE DEFECT, measured on the observe socket in a real browser: an agent's 8 output lines, emitted over 1.6 real seconds, reach the watcher in a single 0.3ms burst 2.9s later — at process exit — followed 23ms afterwards by the turn-end frame that unmounts the progress card.

WHY. chat.Invoke captures stdout into a bytes.Buffer (cmd.Stdout = &stdout, pkg/chat/chat.go:452; runPTY buffers the same way) and writes opt.Stream ONCE after Wait, via emitReduced. Nothing streams while the process is alive. pkg/meet's liveWriter, the fibonacci sampler and the counters are all CORRECT and are simply fed one blob at the end — verified in isolation.

THE USER-VISIBLE COST, which is the reason this is p1 and not a test defect: a human watching a five-minute agent turn sees '0 lines observed / 0 B / 0s elapsed' for five minutes, then the entire answer at once. Bounded live progress exists precisely to prevent that, and it does not work for any real agent, only for the counters.

SECOND DEFECT, same capture: the sampler is not stopped by the turn-end event, so a FINISHED turn keeps emitting text-less heartbeat frames every 5s forever, resurrecting an empty progress card. Fix both together — the heartbeat must stop on spoke.

WANTED. Tee opt.Stream while the process runs (io.MultiWriter into cmd.Stdout for the pipe runner, the existing sink for runPTY) rather than replaying at exit. This is the hot path of EVERY agent invocation, so it needs its own gate and its own arm — do not land it as a drive-by.

GATE. The meet e2e case 'a long Chat reply shows bounded real output and cumulative progress' goes green with its test.fail() marker REMOVED (that marker is the alarm: fixing the stream turns the suite red until the marker is dropped), plus a Go test that asserts Stream receives its first byte BEFORE the process exits.
