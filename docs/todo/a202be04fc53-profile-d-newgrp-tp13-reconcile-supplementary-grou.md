---
id: a202be04fc53
kind: bug
title: 'Profile D newgrp TP13: reconcile supplementary group order'
seq: 179
status: todo
priority: p0
created: 2026-10-02T01:04:45.926264Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Frozen Profile D newgrp:13 FAIL, same Profile C Go result; Profile B GNU provider PASS. The journal compares otherwise equal `id` output: the newgrp shell reports groups 5002,8,5004, while the suite's `ExecAsUser` reference reports 8,5002,5004. Both contain the same membership and primary GID. The suite starts `id` in both contexts; Go `id` was emitting the raw process supplementary vector, so the two credential creation paths exposed different orderings. A focused test reproduced this with the same set in both orders. The candidate makes default `id` place the effective group first only when it is already a supplementary member, as its existing `id -G` path does; native `id`/`newgrp` tests and Linux cross-compilation pass. Exact licensed TP13 replay is required before closing the failure. Keep original codes.
