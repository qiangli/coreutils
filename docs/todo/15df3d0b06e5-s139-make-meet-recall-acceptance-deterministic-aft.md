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

Implemented in 65481b08: await the existing liveJob.finished signal under its mutex before snapshot/cleanup. The manager independently passed 50 focused iterations and full pkg/meet, then the combined non-external Go vet/test gate with the selector fix. Publication acceptance passed.

Delivery accepted 2026-09-08: coreutils PR #8 merged as affb46521ed5bbf2043d86374ad08cc19d4cbf12 (same tree as tested/pinned 6942196e). Native Linux/macOS/Windows CI run 34209811139 passed. Owner final combined Go vet/test and crossvet passed; Meet repair additionally passed 50 focused iterations plus full package. Installed clean Bashy fe53058 at ~/.local/bin/bashy passed deterministic two-live-seat delivery: role seq 3 received by peer; stage seqs 4–8 uncapped with no sprint subscription; invalid selectors append nothing; zero live recipients retains history with an honest receipt. Fixture watchers exited 0 and leases released. Evidence retained at /tmp/s139-installed-candidate-v2 and /tmp/sprint139-delivery/evidence. This is fixture-process delivery evidence, not model inference.
