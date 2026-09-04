---
id: fa2925e2666d
kind: task
title: 'GATED: unify the 164 truncation sites onto the shared reducer'
seq: 46
status: todo
priority: p2
created: 2026-09-04T09:52:41.262112Z
sprint: 123
---

CLOSED until the Yoke gates open. 164 non-test sites across 30 packages disagree today: bashy/pkg/runner/runner.go:66 bounded() is head-only (and is what ycode's harness consumes), supervise/engine.go:46 is tail-only, weave_impl.go:1309 is salience-ranked. A reader cannot tell which discipline produced the text in front of them. Do this only AFTER the shared reducer has a proven contract.
