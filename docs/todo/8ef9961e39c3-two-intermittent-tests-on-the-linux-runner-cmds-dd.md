---
id: 8ef9961e39c3
kind: task
title: 'Two intermittent tests on the linux runner: cmds/dd status output and pkg/foreman control-stop'
seq: 21
status: todo
priority: p2
created: 2026-09-01T14:31:46.174722Z
sprint: 101
---

Two tests fail intermittently on the ubuntu-latest runner and pass on re-run.
Both were observed failing on commit c45a99e4, whose diff is markdown and the
ratchet baseline only -- no code changed -- and both passed on 08e59143 and in
the two pre-ratchet runs 33513814323 / 33515325794.

  cmds/dd      TestDdPOSIXStatusOmitsGNUByteCountExtension
  pkg/foreman  TestServeControlStopCancelsActiveTurn

Locally on darwin, dd passes 5/5. foreman is intermittent HERE too: one
`go test ./pkg/foreman -run TestServeControlStopCancelsActiveTurn -count=5`
invocation reported FAIL for the package while the named test passed 5/5 in the
same run -- so the package-level failure came from something outside the test
body (a leaked goroutine, a TestMain check, or a sibling touched by shared
state), and the next identical invocation was clean. That is the more
interesting half: a package that fails while every test in it passes.

scripts/ci-test-gate.sh now re-runs an unbaselined failure once and reports this
class as FLAKY rather than failing CI, so neither is blocking. That is a
holding position, not a disposition: a test that only sometimes holds is not
evidence either way, and these two are currently carrying no weight.

Wanted: find the shared state. For foreman start from the package-fails-while-
tests-pass observation rather than from the named test. Do NOT close either by
adding a retry loop inside the test.
