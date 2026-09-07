---
id: b0268293747e
kind: task
title: 'pkg/chat: concurrent launch resolution races on agentlaunch.Containerized'
seq: 100
status: todo
priority: p1
created: 2026-09-07T23:03:23.603957Z
---

Found by Sprint 137 independent full-package race gate; reproduced with original origin/main chat.go via Go source overlay, so pre-existing and outside the two assigned teardown defects. Repro: go test -race ./pkg/chat -run ^TestConcurrentOneShotsDoNotCollide$ -count=1. resolveLaunch temporarily assigns and restores agentlaunch.Containerized; concurrent Invoke calls read/write that global while UnsafeLaunchAllowed reads it. Remove per-invocation global mutation using explicit dependency/options injection; preserve launcher policy semantics and test injection. Add race coverage for concurrent chat and direct agentlaunch consumers. No known-failures entry was added. Original two sprint defects remain separately gated; full pkg/chat race cannot be claimed green until this issue is fixed.


Independent source review found the same save/assign/defer-restore pattern in
four chat wrappers (resolveLaunch and argument/unsafe-launch guards). A chat-only
mutex would miss direct agentlaunch consumers such as weave. Production probes
currently have equivalent defaults; the observed race is not evidence of an
unsafe-launch bypass. Fix all four wrappers through per-call dependency/policy
injection and cover concurrent direct/shared callers.
