---
id: 9e6c9b88cee1
kind: task
title: 'bashy apps: a system-wide, read-only Inbox panel for every agent''s waiting mail'
seq: 49
status: done
priority: p1
created: 2026-09-04T17:47:45.966064Z
sprint: 122
closed: 2026-09-04T17:50:35.535046Z
---

`bashy inbox` is first-person: it fixes the filter to one reader and, unless
--peek, advances that reader's cursor. Nobody owns an inbox called
"everybody's", so no invocation of it answers what is waiting across the host,
or which agent is sitting on a backlog. That is a human's question and it had
no surface at all.

Deliver it as a console panel: left nav lists every inbox (viewer first, then
agents, roles, and names the timeline addressed but the catalog does not list);
right pane shows one inbox chronologically with server-side filters and facets.

READ-ONLY IS THE CONSTRAINT, not a preference. bus.SnapshotInbox — the CLI's
read path — opens subscriptions, materializes backlog and advances cursors, all
correct for its own single reader and catastrophic for a page that repaints
every 5s. Add a separate read-only inspection API in pkg/bus and prove it with
a before/after hash of the whole room directory. No write route on the panel.

Gate: go test ./pkg/{bus,webconsole,atlas}, scripts/crossvet.sh,
scripts/ci-test-gate.sh, and /inbox/ added to the console_dom_test.go browser
sweeps.
