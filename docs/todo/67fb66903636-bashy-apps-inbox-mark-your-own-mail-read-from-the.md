---
id: 67fb66903636
kind: task
title: 'bashy apps Inbox: mark your OWN mail read from the web UI, through a nameless route'
seq: 51
status: done
priority: p1
created: 2026-09-04T18:42:52.513728Z
sprint: 122
closed: 2026-09-04T18:42:56.256179Z
---

The web UI is the human-first interface: a person reading their mail in a browser
should finish the act there, not go find a terminal. But the panel is a peek at
every OTHER name, and marking an agent's mail read would leave it durable on the
timeline and never handed over — the worst shape a message store has.

So: exactly one write, POST /api/inbox/read, carrying NO NAME. A {name}/read
route guarded by name==viewer would work until the check was refactored,
mis-cased or forgotten, and the failure is silent. With no parameter there is
nothing to validate. Goes through bus.SnapshotInbox (the same path bashy inbox
uses for its own reader) so the two surfaces cannot drift; mark-one uses
CommitItem so the messages below survive; mark-all acts on the whole inbox, never
the filtered view.

Also: the header pill becomes per-inbox (hidden on your own, PEEK elsewhere) —
a banner contradicting the controls beneath it is worse than none.

Gate: 4 server tests (nameless route, self-only with a byte-level negative on
every other name's files, one-not-the-rest, refusals) verified RED by pointing
the handler at another name; 3 browser tests (controls offered only on your own
inbox, mark-all clears the count and the launcher badge while cairn's message
survives, mark-one leaves the rest).
