# `rmdir` POSIX.1-2016 Issue 7 Audit

Scope: `rmdir [-p] dir...` against The Open Group POSIX.1-2008 Issue 7, 2016
Edition utility page:
<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html>,
audited end-to-end against `cmds/rmdir` as of main `9982454`. GNU-only
behavior (e.g. `-v`, `--ignore-fail-on-non-empty`) is out of the Issue 7
evidence surface and is not pursued here beyond noting it does not
interfere with required semantics.

## 1. Options and Operands

The required `-p` option is implemented. `dir` operands are processed in
argument order; a failing operand is diagnosed on standard error, the final
status is recorded non-zero, and processing continues with the remaining
operands. `--` ends option parsing and a lone `-` after it (or anywhere,
since rmdir takes no option arguments that could be confused with it) is an
ordinary pathname, never standard input.

Operand order is load-bearing, not just "processing continues": an operand
is attempted exactly once, at the position it occupies in argv. Given a
directory and its own child as two separate operands, parent-then-child
leaves the (now-empty) parent in place because it already failed by the
time the child empties it, while child-then-parent removes both.

Evidence: `TestRmdirEmpty`, `TestRmdirContinuesPastErrors`,
`TestRmdirOperandOrderMatters`, `TestRmdirDoubleDashOperand`,
`TestRmdirDashOperand`.

## 2. `-p` Ancestor Walk

`-p` removes the operand, then repeatedly removes its dirname ancestors
left to right (deepest first) until an ancestor removal fails or the walk
runs out of components. The walk stops at the first failure per operand —
including a non-empty ancestor, which is reported like any other rmdir
failure — and never attempts the filesystem root itself (verified by code
inspection of the ancestor-loop termination condition; not exercised by a
live test, since driving a real absolute path up to `/` would touch
directories outside the test's own temp tree). An explicit `./` prefix is
significant: `rmdir -p ./a/b` walks through `.` as a real ancestor and
reports its own removal failure, matching GNU's treatment of an explicit
current-directory component as a pathname component. A trailing slash on
the `-p` operand does not change the computed ancestor chain.

Evidence: `TestRmdirParents`, `TestRmdirParentsExplicitCurrentDirectory`,
`TestRmdirParentsStopsOnNonEmpty`, `TestRmdirParentsWithTrailingSlash`,
`TestRmdirIgnoreNonEmptyWithParents`.

## 3. Dot, Dot-Dot, and Trailing Slash

POSIX's `rmdir()` function mandates `[EINVAL]` when the pathname's final
component is `.`, checked here before any filesystem call (a bare `.` would
otherwise name the working directory itself, and on Windows a trailing single
dot may be stripped during path canonicalization, which could let the removal
spuriously succeed instead of failing as POSIX requires). `.` is checked both in bare form
(`.`) and inside a longer operand (`a/.`, `a/./`), since `filepath.Clean`
would otherwise silently collapse the trailing dot away before the check
ever saw it.

A final component of `..` was previously special-cased to the same hardcoded
EINVAL. Issue 7 instead requires the operation to fail while leaving the
specific errno unspecified. The complete lexical pathname must therefore
reach the host pathname walk: `missing/..` fails at the missing component,
`file/..` fails at the non-directory component, and a valid `child/..` gets
the host's native final-dot-dot result. This was confirmed directly against
this host's `/bin/rmdir`:

```
$ /bin/rmdir "$T/a/.."
rmdir: /tmp/.../a/..: Directory not empty
$ /bin/rmdir "$T/."
rmdir: /tmp/.../.: Invalid argument
```

This was a **confirmed Bashy-owned defect**, fixed in this pass. Removing the
hardcoded EINVAL guard alone was insufficient because `RunContext.Path` uses
`filepath.Join`, which lexically collapses `..` before the filesystem sees it.
`rmdir` now resolves relative operands beneath `RunContext.Dir` with a local
raw-path helper and passes the preserved pathname to both `os.Lstat` and
`os.Remove`. The GNU extension suppresses the result only when the native
error is actually `ENOTEMPTY`/`EEXIST`; it does not suppress invalid-prefix
`ENOENT`/`ENOTDIR` failures.

A trailing slash on an otherwise-removable directory (`d/`) is accepted
and the directory is removed; the diagnostic (when `-v` requests one)
echoes the operand exactly as given, trailing slash included.

Evidence: `TestRmdirDotBare`, `TestRmdirDotDotBareFailsNaturally`,
`TestRmdirRealDirectoryDotDotMayBeIgnored`,
`TestRmdirMissingPrefixDotDotIsNotCleaned`,
`TestRmdirNonDirectoryPrefixDotDotIsNotIgnored`,
`TestRmdirTrailingDotComponent`, `TestRmdirTrailingDotDotComponent`,
`TestRmdirTrailingSlash`.

## 4. Non-Empty / Ignore-Failure Handling

A non-empty directory fails with a `Directory not empty`-shaped diagnostic
and leaves its contents untouched. `--ignore-fail-on-non-empty` suppresses
only `ENOTEMPTY`/`EEXIST` (Windows: `ERROR_DIR_NOT_EMPTY`, which does not
match the POSIX errno constants `errors.Is` checks against, hence the
platform-specific `notempty_windows.go`/`notempty_other.go` seam) — a
missing directory or a non-directory operand still fails and is diagnosed
even with the flag present.

Evidence: `TestRmdirNonEmpty`, `TestRmdirIgnoreFailOnNonEmpty`,
`TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors`,
`TestRmdirIgnoreNonEmptyWithParents`,
`TestRmdirRealDirectoryDotDotMayBeIgnored`,
`TestRmdirMissingPrefixDotDotIsNotCleaned`,
`TestRmdirNonDirectoryPrefixDotDotIsNotIgnored`.

## 5. Permission Failures and Continuation

A directory entry that cannot be unlinked because its parent denies write
permission is diagnosed on standard error, counts toward the non-zero exit
status, leaves the entry in place, and does not abort processing of later
operands (Unix-only; root's permission bypass is skipped for since it
cannot exercise the failure path).

Evidence: `TestRmdirPermissionDeniedContinuesAfterOperand` (Unix-only,
`cmds/rmdir/rmdir_unix_test.go`).

## 6. Standard Input, Output, and Error

Per Issue 7, standard input is not used; `rmdir` reads no interactive
input and has no `-i`-style prompt to justify touching it, exercised with
a `Read` that panics if ever called across a representative mix of a
successful `-p` invocation, a non-empty-directory failure, and a
missing-directory failure. Standard output is not used for the required
interface (the `-v` diagnostic is a GNU extension, written to standard
output only when explicitly requested, and is not POSIX evidence).
Standard error is used only for diagnostic messages; a broken standard
error stream (write failure) does not mask an operand failure — the exit
status still reflects it even though the diagnostic text itself could not
be written, matching the repo's established convention (also used by
`cmds/rm`) that verbose/diagnostic output failures are not specially
handled since they carry no primary command output.

Evidence: `TestRmdirDoesNotConsumeStdin`,
`TestRmdirDiagnosticWriteFailureStillFails`, `TestRmdirVerbose`.

## 7. Exit Status

Exit status is `0` only when every operand (and, under `-p`, every ancestor
attempted) was removed successfully; `1` when any operand or ancestor
failed; `2` for usage errors (missing operand, unsupported flag) —
consistent with the repo-wide documented deviation of using `2` for usage
errors where GNU sometimes uses `1`.

Evidence: `TestRmdirUsageErrors`, `TestRmdirContinuesPastErrors`,
`TestRmdirOperandOrderMatters`.

## 8. Platform Disposition

Unix and Windows are both fully supported: the non-empty-directory
classification is the only platform-conditional piece (`ENOTEMPTY`/`EEXIST`
on Unix, `ERROR_DIR_NOT_EMPTY` on Windows), isolated behind
`notempty_other.go` / `notempty_windows.go`. The dot-EINVAL guard, operand
ordering, `-p` walk, and stdin/stdout/stderr contracts are platform-neutral
Go and are exercised by `go vet` across `linux`, `darwin`, `windows`, and
`freebsd`, plus a cross-compiled build for `aix/ppc64` as an additional
canary.

## 9. Residuals

Diagnostics are fixed English strings; `LC_MESSAGES`/`NLSPATH` catalogs are
not implemented (consistent with the rest of the repo's POSIX-required
surface). The filesystem-root stop in the `-p` ancestor walk is verified by
code inspection and against the loop's termination arithmetic, not by a
live test that reaches an actual `/`, since constructing such a test would
require directory state outside the test's own sandboxed temp tree. `-v` and
`--ignore-fail-on-non-empty` remain supported GNU extensions outside the
required Issue 7 interface. The latter is included in focused regression
coverage because it must not hide native invalid-prefix failures.

## 10. Gate Record

Required local gate for this issue, run on 2026-08-25:

```sh
POSIXLY_CORRECT=1 go test -count=20 ./cmds/rmdir
POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/rmdir
POSIXLY_CORRECT=1 go vet ./cmds/rmdir
go test -count=20 ./cmds/rmdir
go test -race -count=5 ./cmds/rmdir
go test -shuffle=on -count=50 ./cmds/rmdir
go vet ./cmds/rmdir
GOOS=linux go vet ./cmds/rmdir
GOOS=darwin go vet ./cmds/rmdir
GOOS=windows go vet ./cmds/rmdir
GOOS=freebsd go vet ./cmds/rmdir
GOOS=aix GOARCH=ppc64 go build ./cmds/rmdir
./scripts/fmtcheck.sh
bash scripts/applet-test-coverage.sh
python3 scripts/applet-matrix.py --check
python3 scripts/posix_manifest.py --check
go vet $(go list ./... | grep -v /external/)
go test $(go list ./... | grep -v /external/)
```

Results:

* `POSIXLY_CORRECT=1 go test -count=20 ./cmds/rmdir` passed.
* `POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/rmdir` passed.
* `POSIXLY_CORRECT=1 go vet ./cmds/rmdir` passed.
* Default tests (20 runs), race tests (5 runs), and shuffled tests (50
  runs) passed.
* Native and Linux/Darwin/Windows/FreeBSD vet passed; `GOOS=aix
  GOARCH=ppc64 go build ./cmds/rmdir` passed.
* `./scripts/fmtcheck.sh` passed (2016 files).
* `bash scripts/applet-test-coverage.sh` passed (154 shipped packages).
* `python3 scripts/applet-matrix.py --check` passed after updating rmdir's
  generated count to two test files and 29 named top-level tests.
* `python3 scripts/posix_manifest.py --check` still fails on the pre-existing
  `sh: partial state requires focused semantic evidence` finding, unrelated to
  rmdir. The TSV and rendered Markdown rmdir entries were updated together.
* Repository-wide `go vet` (excluding `external/`) passed with no findings.
* Repository-wide `go test` (excluding `external/`) — see the "Default
  regression" note below.

Behavioral confirmation against the real host `rmdir(1)` (Darwin,
`/bin/rmdir`), used to validate the `.`/`..` distinction fixed in this
pass:

```
$ /bin/rmdir "$T/a/.."
rmdir: /tmp/.../a/..: Directory not empty
$ /bin/rmdir "$T/."
rmdir: /tmp/.../.: Invalid argument
```

Default regression: repository-wide `go test` excluding `external/` ran to
completion (`cmds/rmdir` itself passed). The only failure anywhere in the
run was `cmds/getconf#TestDarwinAdapterMatchesEverySafelyQueryableValue`,
comparing `STREAM_MAX`/`OPEN_MAX` against the host's live `getconf` output;
this session's shell has `ulimit -n` raised to 92160 instead of a
conventional default, which the host's own `getconf` reflects and this
repo's fixed-table implementation does not. It is unrelated to `rmdir`,
touches no file this change modified, and is an environment/ulimit
artifact of this shell session, not a defect introduced here.
