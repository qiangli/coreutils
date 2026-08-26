# Go evidence Linux Profile C reconciliation (issue 782)

This reconciliation makes the canonical `go_evidence` lane executable on the
non-root Ubuntu/Linux Profile C target. It changes evidence selection, not the
116-command ownership inventory or any evidence state. A reference in the Go
lane must name a top-level public test that produces exact `run` and `pass`
events on that target; a Windows, Darwin, non-Linux, skipped integration, or
unavailable-host differential is not target evidence.

## New focused evidence

The ledger appends 68 POSIX-focused top-level TestIDs added after the Sprint 79
baseline. They cover `csplit` (2), `expr` (3), `grep` (5), `join` (6), `ls`
(2), `pax` (16), `sed` (1), `sort` (8), `tr` (10), `unexpand` (5), `uniq`
(6), and `wc` (4). Ten contemporaneous tests remain outside `go_evidence`:
GNU `grep -o`, GNU `join -i` and its private-provider guards, the recorded
`tr` collation residual, non-POSIX compatibility guards for `tr` and `uniq`,
and GNU `wc -L`. `TestJoinCPOSIXAndIgnoreCaseBypassCollator` remains evidence
because its C and POSIX subtests directly prove the required byte-order path;
its extension subtest is additional coverage rather than the sole assertion.

## Target-invalid references removed or repaired

- The Windows-only references on `chgrp`, `chmod`, `chown`, `logger`, `mkdir`,
  `more`, `newgrp`, and `uname`, the non-Linux `ps` reference, and the
  Darwin-only `tty` reference are removed. Their target-relevant clauses remain
  exercised by the existing general, Unix, injected-provider, and Linux tests
  in the same rows; platform refusal behavior is not evidence for Ubuntu.
- `getconf` drops two host-command differentials, two Darwin adapter tests, and
  the Windows refusal test. Its inventory, minimums, error/output, option,
  arity, and Linux-derived-value TestIDs remain explicit.
- `logname` drops three live-login integration tests that skip when a container
  has no usable login UID. The retained `TestResolveLoginUID`,
  `TestLoginNameHasNoEffectiveUserFallback`, `TestLognameNoLoginName`, and
  injected output/error test cover the required lookup, no-fallback, failure,
  and output clauses hermetically. A real login-session probe remains an
  integration boundary rather than canonical Go evidence.
- `mkdir` drops the filesystem-dependent inherited-setgid test, which skips on
  overlay filesystems. The already-cited hermetic
  `TestMkdirVirtualUmaskCorrectionPreservesInheritedSpecialBits` exercises the
  same special-bit preservation rule through the injected filesystem seam.
- The `cp` symlink-preservation test was accidentally named
  `*_linux_darwin_test.go`; Go interpreted the final filename suffix as
  Darwin-only despite its `darwin || linux` build expression. It is renamed to
  `*_unix_test.go`, and its pathname assertion now compares file identity so
  Linux's `/proc/self/fd` virtual-working-directory spelling is accepted only
  when it identifies the intended destination symlink.
- `pr` drops `TestPRPOSIXLYCorrectDifferentials`. GNU `pr` under
  `POSIXLY_CORRECT=1` still centers the header and pads merged `-s` fields, so
  that differential contradicts the Issue 7 formats. The retained hermetic
  `TestPRDefaultPageStructure` and
  `TestPRMergeAssumesPOSIXTabExpansionAndReplacement` cover those clauses.

## Source defect found by the target lane

`nice` keeps `TestNiceUtilityStartsAtAdjustedPriority`. On Linux,
`x/sys/unix.Getpriority` exposes the kernel syscall result (`20 - nice`) rather
than libc's converted niceness. The former shared Unix implementation passed
that raw result into `Setpriority`, so a default-priority process requested an
effective niceness of 20 instead of 5 and the kernel clamped it to 19. Linux
now converts the raw result before applying the requested adjustment; other
Unix targets retain their native wrapper.

## Exact gate

The acceptance run uses a non-root `golang:1.26-bookworm` container, mounts the
public umbrella read-only so the sibling `sh` replacement resolves, disables
network access, and exports `POSIXLY_CORRECT=1` globally. This command shape is
the required semantic gate inside the container:

```sh
python3 scripts/posix_interface_runner.py \
  --state-dir /state/run --owner go --json
```

The issue-782 result is 78 command events passed, 1,077 exact TestIDs passed,
and zero missing, skipped, or failed TestIDs. No proprietary suite is involved.
