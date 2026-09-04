---
id: 8aace43b4d43
kind: task
title: 'weave: a naive tail cut silently undoes weaveTrimVerifyOutput''s salience ranking'
seq: 41
status: todo
priority: p1
created: 2026-09-04T09:52:41.148163Z
sprint: 123
---

OPEN CLASS — shippable before the Yoke gates, but requires ack with the #98/#100 holders first (shared test gate). pkg/weave/weave_impl.go:1352. weaveRunVerify returns output ALREADY salience-trimmed to 2000 bytes at :1287, which places FAIL/--- FAIL/panic: lines at the HEAD. weaveCollectVerifyEvidence then appends the dirty-tree note and does 'vo = vo[len(vo)-2000:]' — a tail cut that discards exactly those head lines. On a dirty working tree with large verify output the ranking is silently undone. Same 'result survives, evidence discarded' defect the file's own comment says cost two multi-hour diagnoses. Gate: a test with >2000 bytes of output AND a dirty tree that asserts the FAIL line survives.
