---
id: 8ef9961e39c3
kind: task
title: 'pkg/foreman: the PACKAGE fails intermittently while every test in it passes'
seq: 21
status: todo
priority: p2
created: 2026-09-01T14:31:46.174722Z
sprint: 101
---

`pkg/foreman TestServeControlStopCancelsActiveTurn` failed on ubuntu-latest in
Actions run 33518958296 -- a commit whose entire diff was markdown plus the
ratchet baseline, so no code changed -- and passed on 08e59143 and in the two
pre-ratchet runs 33513814323 / 33515325794.

The interesting observation is local, on darwin. One invocation of

    go test ./pkg/foreman -run TestServeControlStopCancelsActiveTurn -count=5

reported FAIL for the PACKAGE while the named test printed `--- PASS` all five
times, and the next identical invocation was clean with rc=0. A package that
fails while every test in it passes means the failure came from outside the test
bodies: a leaked goroutine outliving its test, a TestMain check, or shared state
touched after the test returned.

Start from that, not from the test name. Do NOT close this by adding a retry
loop inside the test.

Not baselined. scripts/ci-test-gate.sh re-runs an unbaselined failure once and
reports this class as FLAKY, so it blocks nothing -- a holding position, not a
disposition.

The POSIX half of this story (cmds/dd status output, a conformance rule on a
certified utility) was split out to 5db8d65e under the POSIX cert sprint.
