---
id: 15df3d0b06e5
kind: task
title: 'S139: make Meet recall acceptance deterministic after cancellation'
seq: 104
status: done
priority: p1
created: 2026-09-08T09:09:12.963571Z
weave: 3
assignee: sprint139-manager
sprint: 139
closed: 2026-09-08T09:40:22.277502Z
---

Delivery blocker: repeated full integration gates at coreutils88aa3f47 + selector7a80dd61 fail TestRecallOverHTTPCancelsBeforeTheAppendAndRetractsAfter, while isolated one-shot passes. Last full run had only this failure. Diagnose exact assertion with repeated focused runs and retain failure details. Suspected race: canceled address job finalization appends after the test snapshots transcript length for an unknown recall. If confirmed, wait for actual job completion before measuring unknown-handle side effects; preserve all recall/retraction/append-only assertions, no sleeps or skips. Scope Meet test/necessary narrow seam only. Other source unchanged. Manager retains final gates and publication.

Implemented in 65481b08: await the existing liveJob.finished signal under its mutex before snapshot/cleanup. The manager independently passed 50 focused iterations and full pkg/meet, then the combined non-external Go vet/test gate with the selector fix. Publication acceptance remains open.
