---
id: f5e7b865437c
kind: task
title: 'Meet web UI: hide room actions, name the room''s owner, and default the recipient to it'
seq: 39
status: done
priority: p1
created: 2026-09-04T09:19:42.53618Z
sprint: 122
closed: 2026-09-04T09:20:47.402297Z
---

Four changes to the Meet app, plus the route rename.

1) ROOM ACTIONS HIDDEN. round/poll/ask/converge/mark and the two pointer menu items are gone from the composer. They were built for a facilitator driving a floor; a person typing in a chat window is not that. The verbs stay on the CLI. Asserted as an ABSENCE (e2e), because nothing else fails when a menu quietly returns.

2) THE OWNER IS NAMED. New server-side projection pkg/meet/owner.go: roomView adds a resolved owner + owner_title to the state a browser reads. DefaultTo (the late-bound seat label, e.g. conductor:99) outranks Chair, because unaddressed mail already lands on that seat. The title comes from role.Title, so a sprint room says 'project manager' and an ordinary meeting says 'facilitator'. It is a PROJECTION, never a stored field — State is persisted with the same marshaller and a copied holder is the exact bug the late-bound label prevents. A vacant seat reports an empty name and a real title.

3) RECIPIENT DEFAULTS TO THE OWNER AND PERSISTS. A Send-to dropdown replaces the actions menu, preselecting the owner; 'Everyone' is a real choice. The pick is stored per room in localStorage (per browser: two people in one room may address different agents, so it must not be written onto shared room state). A typed @name still overrides for one message. The owner is added to the recipient list and the roster when it is not seated — a facilitator runs the floor without being on the roster, so the one preselected name was the one name a reader could not choose back.

4) /relay -> /meet. The app was renamed to Meet everywhere but the address bar. Mount is now /meet/; the verb stays 'relay' (same split the board made when it became /sprint/). /relay and /relay/ redirect. --disable now matches the public mount as well as the internal name: it matched the name only, so 'bashy apps serve --disable meet' was a SILENT NO-OP that left the panel listed, routed and reachable.

Found while working: the Playwright harness passed -tags meetspa, a tag nothing consumed since the default embed moved to artifact/ — so it built the SPA, discarded it, and asserted against the committed bundle. Restored embed_spa_dist.go behind that tag.

GATE: 5 pkg/meet owner tests, 3 pkg/webconsole route/disable tests, 16/16 Playwright (3 new), full verifydom browser suite, crossvet PASS, ci-test-gate OK (6915 passed).
