---
id: 5db8d65e2518
kind: task
title: 'dd: POSIX status output is intermittently wrong on the linux runner'
seq: 23
status: wontfix
priority: p2
created: 2026-09-01T15:04:03.516013Z
closed: 2026-09-30T15:33:53.58095Z
---

`dd` is a POSIX-required utility in the certification scope, and this test
asserts a POSIX Issue 7 conformance rule directly: with POSIXLY_CORRECT present
(even empty), the GNU "N bytes copied" line must be suppressed and the status
must be exactly the two POSIX lines.

    cmds/dd  TestDdPOSIXStatusOmitsGNUByteCountExtension

It failed once on ubuntu-latest, in Actions run 33518958296 -- a commit whose
entire diff was markdown plus the ratchet baseline, so no code changed -- and
passed on 08e59143 and in the two pre-ratchet runs 33513814323 / 33515325794.
Locally on darwin it passes 5/5.

CERT RELEVANCE: an intermittently-correct POSIX status output is worse than a
consistently wrong one. A conformance arm that samples this once can record a
PASS that the next run would not reproduce, and the same nondeterminism could
just as easily surface inside a licensed suite as an unexplained TP failure.
Resolve the mechanism before quoting any dd status result as evidence.

NO CONFIRMED CAUSE YET, and the failure message was not captured: at the time
scripts/ci-test-gate.sh did not print test output. It does now (77d055c2), so
the next occurrence will carry the actual assertion diff -- start there rather
than guessing.

Already ruled out:
  * ambient environment. dd reads rc.Env via envPresent(), not os.Getenv, and
    the test passes an explicit env slice with no os.Environ() fallback, so an
    inherited POSIXLY_CORRECT cannot reach it.
  * intra-package parallelism. No cmds/dd test calls t.Parallel().
  * signal cross-talk from dd_signal_unix_test.go. It drives a re-exec'd helper
    SUBPROCESS, so its signals are not delivered to the test process.

Not baselined: it is green far more often than not, and a baseline is for known
failures rather than noise. The gate re-runs it and reports FLAKY, so it blocks
nothing while it stays unexplained.

Split from 8ef9961e, which keeps the non-POSIX half (pkg/foreman).

## Review 2026-09-30 (steward)

- Status: unknown. No dd code change since 2026-09-01 and no recorded recurrence found in commit history; the ci-test-gate FLAKY report (output capture since 77d055c2) is where a recurrence would show. Sibling 8ef9961e was closed in Sprint 137 with a separate root cause (pkg/foreman), which does not explain this one.
- Outdated: nothing structural; Go 1.26 container reference is now Go 1.27.1 for the certification build.
- Next step: search the coreutils Actions history since 2026-09-01 for `TestDdPOSIXStatusOmitsGNUByteCountExtension` FLAKY; if none, run it `-count=200 -race` on a Linux runner (ordinary user). If it never reproduces, record that with run IDs and close; if it does, fix from the captured diff.
- Acceptance: either a root-caused fix with a regression test, or a recorded no-reproduction (run IDs + counts) and the dd:* identities PASS in the fresh baseline arm.
- Priority p2; independent of the licensed host; can run anytime.
