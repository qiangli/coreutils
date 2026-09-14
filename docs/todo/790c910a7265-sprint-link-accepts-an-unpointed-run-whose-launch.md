---
id: 790c910a7265
kind: task
title: sprint link accepts an unpointed run whose launch is explicitly bounded
seq: 137
status: todo
priority: p1
created: 2026-09-14T14:17:51.931611Z
sprint: 175
---

Gap found by Sprint 171: story points derive the run's wall-clock cap (8 pts = 30 m) and weave start rejects a larger explicit --max-runtime, so a lane that legitimately needs 2h30m must be UNPOINTED - and sprint link refuses unpointed runs, so the sprint's token totals and roots-per-100k read 'missing'. Fix (MVP): keep the invariant 'linking promises a bounded execution' but let an explicit --max-runtime satisfy it: an unpointed run in a non-todo state with LaunchSpec.MaxRuntime > 0 is linkable; unpointed todo runs and unbounded launches stay refused (message names both ways out: points, or launch with --max-runtime then link). Pointed behaviour unchanged. Test in pkg/weave/weave_story_points_test.go.
