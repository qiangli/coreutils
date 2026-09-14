---
id: ab7e1b2880d1
kind: task
title: managed-session inbox relay full-scans the bus timeline every second and burns a core (foreman serve observed)
seq: 127
status: done
created: 2026-09-13T15:59:28.216066Z
assignee: rabbet
sprint: 156
closed: 2026-09-14T07:00:56.038379Z
closed_by: rabbet
---

ORIGINAL REPORT (2026-09-13 15:27–16:00Z, dev box, dhnt checkout): `bashy foreman serve sprint-165-manager` (a foreman-spawned Claude manager session) ran at 66–126% of a core for 30+ min while its child `claude` sat at 2%. The resource-observer posted host CPU pressure 100% / 92% warnings to the sprint #165 seat because of it. The foreman's PTY transcript (~/.bashy/foreman/<id>/log) grew ~590 B/s while the child only drew its TUI spinner (5,600+ cursor show/hide toggles in 31 min). Original hypothesis: per-write cost in the PTY relay (full-buffer re-parse or busy poll on the pty read side).

DIAGNOSIS (2026-09-14, investigated on the same host): the hypothesis names the wrong mechanism. The PTY relay is not the cost; the session's 1-second UNIFIED-INBOX RELAY POLL is, and it is the Sprint 138 watcher-CPU defect half-fixed.

Measured:
- agentpty.Run hosting a fake TUI spinner (cursor toggles, 3 Hz, Capture:true, ctlsock on): 0.0% CPU. At 50 Hz / ~300 writes/s: 0.9% / 2.6%. The relay is innocent at any realistic rate.
- room.Timeline(0) on this host (~/.bashy/room/timeline.jsonl = 26.8 MB, 246,206 lines): 217 ms per call.
- bus.PrepareForAgent("sprint-165-manager", "") — the coreutils half of ONE relay tick: ~435 ms (two Timeline(0) reads: ResolveFor + UnreadNotifications).
- bashy inbox --as sprint-165-manager --peek — the same unified scan the bashy PrepareTurnInbox hook runs (bashy startup is 20 ms): ~3.4 s.
A time.Ticker(1s) whose body takes 0.4–3.4 s fires back-to-back with no idle gap: 100% duty cycle, independent of what the child prints.

Code path:
1. chat.Start -> s.startInboxRelay (pkg/chat/session.go) -> runInboxRelay with managedInboxPoll = 1s (pkg/chat/inbox_relay.go), calling bus.PrepareForAgent UNCONDITIONALLY every tick.
2. PrepareForAgent -> SnapshotInbox (pkg/bus/inbox.go) -> ResolveFor -> room.Timeline(0) (pkg/bus/resolve.go) AND UnreadNotifications -> watchTimeline(0) (pkg/bus/inbox.go). room.Timeline is os.ReadFile + json.Unmarshal of every line since the archive watermark — no offset, no stat gate.
3. Then the host hook bus.PrepareTurnInbox (bashy internal/agentos unifiedTurnPreamble -> snapshotUnifiedInbox) scans the mb board and every meet room the agent belongs to.

Precedent: bashy internal/agentos/inbox_poll.go (commits 58dde5c, 7bba76e, 2e31f33) fixed exactly this for the CLI `bashy inbox --watch` with an fsnotify + fingerprint gate and backoff. The in-process relay in coreutils/pkg/chat never received the gate.

Blast radius — every long-lived managed session, not just foreman:
- AFFECTED: bashy foreman serve (pkg/foreman/steer.go -> chat.Start; includes the managed-sprint manager); bashy meet participant seats (pkg/meet/steer.go -> one relay PER SEAT); bashy herald ACP agents (pkg/herald/acp.go; relay gated on acp.idle, which is most of the time); bashy steward supervise (chat.Start PLUS bus.NewSidecar(0) at a 1 s poll reading Timeline(0) per subscription — two hot loops); bashy chat interactive/coach (pkg/chat/interact.go runs the same runInboxRelay in the foreground path). Session.Say and ACP start pay one full scan per turn (0.4–3 s latency per steer, not continuous).
- NOT AFFECTED: one-shot bashy invoke (pkg/chat/chat.go: no relay, no bus call in the loop); weave workers (pkg/weave/pty.go: agentpty.Run directly; weave_notice.go reads Timeline(0) once per state change only); bashy inbox --watch (the fixed one) and bus wait (stat-before-read).

Secondary findings (real, not the cause):
- trustClearTap.Write (pkg/agentpty/pty.go) reallocates an 8 KiB tail string and runs ClassifyGate on EVERY PTY write — measured 2.6% at 300 writes/s.
- The rolling archive has only rolled through August; September alone is 27 MB / 246k events, so all 14 Timeline(0) call sites on the host pay for it. The cost scales with host age, which is why it surfaced now.

Acceptance (corrected): a managed session (foreman/meet/herald/steward/chat interact) whose agent is idle stays under 2% CPU over 60 s (measure with `ps -o pcpu`) on a host with a multi-MB timeline; inbox delivery latency is unchanged when a message arrives (the relay still delivers within its poll period); the transcript is still complete.
