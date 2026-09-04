---
id: 368572a99ca7
kind: task
title: 'bashy apps launcher: unread badge on the Inbox tile, counting the viewer not the fleet'
seq: 50
status: done
priority: p1
created: 2026-09-04T18:03:33.947164Z
sprint: 122
closed: 2026-09-04T18:03:42.660931Z
---

The panel is a PEEK: looking at an agent's inbox advances nothing, so that agent's
backlog falls only when the agent itself reads. A fleet total on the tile would
therefore be a number the person looking at it can never clear, and a badge that
never clears is one people learn to ignore. Badge = viewer_unread; the fleet
figure goes in the tooltip, spread over the inboxes that actually hold something.

The peek is also what makes it cheap. A pure read needs no per-viewer state, so
ONE timeline parse answers for every inbox at once — bus.Inboxes, revalidated by
stat rather than a TTL (a TTL under the 5s poll misses every request; one over it
serves a count from before the message being asked about). Measured 161k events /
18 MB / ~176ms that three pollers were each about to pay separately.

Badge data gets its own route (?summary=1), NOT a field on /api/apps: that path
is consoleWidePath, so per-panel counts there would hand a session scoped to the
Terminal alone the fleet's mail volume.

Gate: package tests + two new browser tests asserting the badge is the viewer's
count and is ABSENT when nothing is waiting for them.
