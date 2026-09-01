---
id: 9f98d7080d45
kind: task
title: 'webconsole: phone pairing is an action, not an on/off setting'
seq: 26
status: todo
priority: p1
created: 2026-09-01T19:06:58.158276Z
assignee: ci-repair
sprint: 101
---

The Settings section carried an 'Enable phone pairing' switch. A switch implies a persistent setting; a pairing pass is a single-use, time-boxed credential, and it is wanted once. Operator direction: always show the section, drop the on/off control, make it read like the Background section.

Now: heading + hint + a 'Show pairing code' action button (the Favorites idiom), always present, minting on an explicit click so opening Settings never arms LAN access by itself.

Note the QR was never broken -- a QR can only exist for a LAN-reachable address, so a loopback console legitimately has none, and --pair refuses a loopback bind outright. Verified in Chrome with pairing armed: the panel renders data:image/png;base64 after the click.
