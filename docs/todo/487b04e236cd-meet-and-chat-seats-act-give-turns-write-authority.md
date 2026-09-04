---
id: 487b04e236cd
kind: task
title: 'Meet and Chat seats act: give turns write authority'
seq: 57
status: done
priority: p1
created: 2026-09-04T20:42:49.056508Z
sprint: 122
closed: 2026-09-04T20:43:27.06159Z
---

A seat runs a sprint or drives the machine as steward, so a read-only turn made the surface a suggestion box. Turns launch with ReadOnly=false + AllowUnsafe, one policy in turnAuthority, restorable host-wide with BASHY_MEET_READONLY=1.
