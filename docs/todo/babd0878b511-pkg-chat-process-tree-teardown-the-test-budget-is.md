---
id: babd0878b511
kind: task
title: 'pkg/chat process-tree teardown: the test budget IS WaitDelay, and hitting the fallback means the group kill missed a pipe holder'
seq: 19
status: done
priority: p1
created: 2026-09-01T14:02:20.869035Z
sprint: 137
closed: 2026-09-07T23:16:39.0342Z
---

CI-blocking and INTERMITTENT. macos-latest: PASSED in GitHub Actions run
33513814323, FAILED in 33515325794. Passes 3/3 locally on darwin.

Observed:
  --- FAIL: TestCancelKillsDescendants (5.02s)
      proctree_test.go:139: Run did not return after cancellation

The 5.02s is the whole finding. The test budgets exactly 5s for Run to return
after cancel(); chat.go sets cmd.WaitDelay = 5 * time.Second as the FALLBACK for
when the process-group kill fails to close the inherited pipes. So the test
deadline and the implementation's fallback are the same number, and the test
loses the race by milliseconds whenever the fallback path is taken.

Two separable defects:

1. Test: a single 5s budget conflates "did it hang" with "was it prompt". Split
   them -- wait generously for return (hang detection), then assert elapsed is
   comfortably under WaitDelay (promptness), the way the sibling test at
   proctree_test.go:100 already asserts elapsed < 3s. A failure then reports
   WHICH property broke instead of "did not return".

2. Product (the reason it is intermittent at all): returning at ~WaitDelay means
   killProcessTree did NOT unblock Wait on that host -- a descendant kept the
   stdout/stderr pipe open through the group SIGKILL. Worth confirming whether
   kill(-pid) returned ESRCH and fell back to cmd.Process.Kill() (which reaches
   only the already-dead direct child, leaving the grandchild holding the pipe).
   That fallback in proctree_unix.go killProcessTree is the suspect.

Do not "fix" this by enlarging the test budget alone: that would hide defect 2,
which is the one that strands descendants in production.

Verify with: go test ./pkg/chat -run TestCancelKillsDescendants -count=10

## 2026-09-04 — probe results (darwin dev host, NOT the CI runner)

The suspected fallback was NOT observed here. An instrumented copy of
`killProcessTree` counting which branch it takes, 150 iterations under eight
busy CPU burners:

    group-kill ok=150   bare-pid fallback=0   slow(>500ms)=0   worst=0.007s

So on this host `kill(-pid)` never returns ESRCH, the grandchild never orphans,
and Wait returns ~1 ms after cancel — three orders of magnitude under WaitDelay.
An unmodified `-count=20` also passes.

What that does and does not settle:

  * It does NOT clear defect 2. A negative on a host that has never reproduced
    the failure is not evidence about the host that does — the failing runs are
    both macos-latest GitHub runners, and the difference is the runner, not the
    code path. Do not close this on a local green.
  * It DOES say the ESRCH-fallback hypothesis cannot be confirmed by local
    reproduction, so the next step is instrumentation that survives to CI:
    record the branch and the errno at the point of the kill and print it on
    failure, rather than trying to provoke it here.

Defect 1 (the test budget equalling WaitDelay) is independent of all of this and
is still worth fixing first: while the two numbers are the same, a genuine
defect-2 failure and a merely slow runner produce the SAME message, so CI cannot
tell them apart — which is why this entry has stayed a `sometimes` for three
days without an answer.


## 2026-09-07 — Sprint 137 root-cause fix and independent reproduction

The direct child's exit ended os/exec's cancellation watcher while Cmd.Wait
could still be draining pipes inherited by descendants. The new real-process
TestCancelKillsDescendantsAfterLeaderExit waits until the wrapper is reaped,
then cancels. The sprint owner ran it with original chat.go via a Go source
overlay: cancellation took 4.995579292s and the grandchild survived. This is
positive reproduction of a product defect, not a green-run inference.

The fix owns cancellation until Wait completes, retries group signals while
teardown remains pending, and stops permanently on ESRCH to avoid signaling a
reused group number. It retains signal/fallback errno and Wait errors, with
separate 10s hang detection and a 3s promptness assertion. WaitDelay remains 5s.
A controlled missed-signal fixture verifies retries; additional tests verify
ESRCH cessation and fallback diagnostics. The baseline entry is deleted.

The original macOS CI log did not record signal outcomes, so its exact failure
cannot be attributed conclusively to ESRCH or a particular kernel race. That
historical uncertainty is preserved. Owner focused tests passed ten repetitions
on Darwin; macOS CI verification remains pending.

The expanded full-package race check independently exposed a PRE-EXISTING
launcher-policy global race, reproduced against original chat.go and recorded
separately as b0268293747e. No baseline entry was added for it. The full-package
race run must not be described as green; targeted lifecycle race checks and
ordinary full-package checks are separate evidence.


## Closure verification — 2026-09-07

Actions run [34168966573](https://github.com/qiangli/coreutils/actions/runs/34168966573)
passed the complete macOS, Ubuntu, and Windows legs on delivery commit b1e900e4.
The owner downloaded both Unix process-lifecycle JSON artifacts: each contains
150 passing events (15 tests, 10 repetitions each), no failing events, and ten
passes of both TestCancelKillsDescendants and
TestServeControlStopCancelsActiveTurn. Local verification also passed 234
ordinary affected-package tests, 150 race-enabled lifecycle cases, and the full
crossvet scope. No baseline was added; the chat entry was deleted.

Closure addresses the independently reproduced lifecycle defects described
above. It does not claim that the historical logs identify an exact signal
branch or fully reconstruct the original intermittent event.
