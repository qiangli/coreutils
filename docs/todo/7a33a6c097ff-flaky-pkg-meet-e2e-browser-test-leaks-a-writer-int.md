---
id: 7a33a6c097ff
kind: task
title: 'FLAKY: pkg/meet e2e browser test leaks a writer into its TempDir teardown'
seq: 105
status: todo
priority: p2
created: 2026-09-09T02:04:26.587122Z
---

Reported by scripts/ci-test-gate.sh during the sprint 140 closure gate on 2026-09-09, against clean main bd3adf98 (no local code changes):

  --- FAIL: TestE2EAddressAnAgentAndTheReplyReachesTheBrowser (0.27s)
      testing.go:1464: TempDir RemoveAll cleanup: unlinkat
      .../TestE2E.../001/2026-07-08-chat-room-5a64/seen: directory not empty

  gate: FLAKY -- failed in the suite, PASSED on an isolated re-run.

The failure is in CLEANUP, not in the assertions: the test body passes, then t.TempDir teardown cannot remove the room directory because something is still creating files under seen/ while RemoveAll walks it. 001 is the first TempDir newRoom creates, which is BASHY_MEET_DIR, so this is the meet store rather than the room store.

WHAT IT MEANS: a goroutine outlives the test. seen/ holds per-reader cursors, so the surviving writer is a subscriber or served connection still advancing a cursor after the test body returned. serveTest/dialObserve start real machinery; if any of it is not joined before cleanup, the race is inherent and will keep recurring at whatever rate the machine's timing produces.

WHY IT MATTERS BEYOND THE NOISE: the ratchet is explicit -- "a test that only sometimes holds is not evidence". This one currently passes the gate by being re-run in isolation, which means a REAL regression in the same teardown path would be indistinguishable from this flake and would be waved through the same way.

NOT A BASELINE CANDIDATE: do not add it to test/known-failures.txt. The baseline only shrinks, and an intermittent is not a known failure -- it is an unjoined goroutine with a fixable owner.

FIX DIRECTION: make the test join what it starts. Whatever serveTest and dialObserve spawn should be shut down and waited for in a t.Cleanup registered BEFORE the TempDir it writes into, since cleanups run LIFO -- a cleanup registered after t.TempDir runs before the directory removal, which is the ordering this needs.

Filed from the sprint 140 closure gate; it is not sprint 140 work and carries no sprint.
