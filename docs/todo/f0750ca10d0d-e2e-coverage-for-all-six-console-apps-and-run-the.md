---
id: f0750ca10d0d
kind: task
title: E2E coverage for all six console apps, and run the suite in CI
seq: 55
status: done
priority: p2
created: 2026-09-04T20:10:41.69959Z
sprint: 122
closed: 2026-09-04T20:10:59.991338Z
---

The e2e tag covered launcher/files/meet/terminal only, and nothing ran it: sprint, inbox and messages had no wire-level test at all, and the files scope assertion was guarded by 'if code == 200' against a request that always 401s, so it never ran. Add per-app coverage (sprint board API, inbox peek-does-not-consume + nameless mark-read, messages round trip with derived sender, asset plumbing for every page, console chrome placement) and wire go test -tags e2e into CI.
