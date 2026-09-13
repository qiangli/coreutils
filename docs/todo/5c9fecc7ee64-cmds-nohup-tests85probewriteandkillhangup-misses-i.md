---
id: 5c9fecc7ee64
kind: task
title: 'cmds/nohup: TestS85ProbeWriteAndKillHangup misses its 2s spawn deadline under the full-suite gate (pre-existing; blocks every weave pull)'
seq: 116
status: done
priority: p1
created: 2026-09-13T00:05:00.011651Z
sprint: 161
closed: 2026-09-13T00:06:52.944647Z
---

Measured 2026-09-12: the repo gate (go test over every non-external package) fails on the pristine base 88a8d965 with '--- FAIL: TestS85ProbeWriteAndKillHangup (2.60s)' while the package alone and the test alone pass (5.5s / 1.0s). The PID-file read loop gives the nohup child 2s to be spawned by /bin/sh; under full-suite process-spawn contention on macOS that is exceeded. Fix: widen the spawn and exit deadlines with a comment; no product change. Found by sprint 161 wave-1 pulls (all three refused on this test).
