---
id: a754705c3c7e
kind: task
title: S183.misc — remove mandatory model review from Bashy workflows
seq: 143
status: assigned
priority: p0
created: 2026-09-15T01:13:30.97491Z
weave: 25
assignee: qiangli
sprint: 183
---

Emergency fix: remove mandatory model review and test requirements from Bashy workflows. Follow Unix/KISS boundaries: each orchestration command does its core job; extra assurance, cost, and policy are opt-in. Configured verify, suite, clean-room, and isolation gates remain honored, but absent gates do not block clean committed work. Explicit standalone judge/pair tools and opt-in --review-agent remain available, while a normal pull, autopilot, or salvage neither invokes nor requires a model review, and stale failed pair evidence cannot poison a later merge. Keep compatibility flags where cheap. No model judge for this story.
