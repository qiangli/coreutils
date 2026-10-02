---
id: 4b42d2a2a229
kind: bug
title: 'pax: stream large PAX archive without whole-archive memory buffer'
seq: 177
status: todo
priority: p0
created: 2026-10-02T00:05:03.403118Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Profile D TP241, POSIX.cmd/pax assertion #343 (size extended header for >=8 GiB), failed in the frozen diagnostic. The fixture leaves a sparse 8 GiB source and a zero-byte archive. At 2026-10-01 22:07:42 UTC the Linux kernel OOM log identifies the killed process as pax (PID 872321, anon RSS 3.36 GiB). Coreutils cmds/pax/modes.go writeMode buffers the entire tar payload in bytes.Buffer before post-processing, so this implementation cannot write an 8 GiB member on the 4 GiB droplet. Preserve the frozen raw UNRESOLVED result. Design a bounded-memory streaming write path that retains PAX header modifications, add a sparse large-file regression with bounded resident memory, and replay exact licensed TP241 on a pinned candidate after original D evidence is archived. Do not rely on increasing droplet RAM or relabeling the raw result.
