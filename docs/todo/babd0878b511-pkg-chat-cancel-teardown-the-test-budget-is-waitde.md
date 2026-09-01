---
id: babd0878b511
kind: task
title: 'pkg/chat cancel teardown: the test budget IS WaitDelay, and hitting the fallback means the group kill missed a pipe holder'
seq: 19
status: todo
priority: p1
created: 2026-09-01T14:02:20.869035Z
sprint: 101
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
