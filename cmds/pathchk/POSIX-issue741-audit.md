# `pathchk` POSIX Issue 7 filesystem audit

Normative source: [POSIX.1-2016 Issue 7 `pathchk`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html).

## Accepted behavior and evidence

Default mode checks pathname and component lengths against the containing
filesystem, diagnoses unsearchable and non-directory prefixes, preserves the
kernel resolution order for constructs such as `symlink/..`, and continues
across operands while aggregating status. `-p` substitutes the portable
minimum limits and character set; `-P` adds empty-name and leading-hyphen
checks.

Filesystem limits are queried at the initial containing directory and again
after each existing directory component. Darwin uses
`pathconf(_PC_PATH_MAX/_PC_NAME_MAX)`. Linux uses the pathname-copy ABI limit
and `statfs.f_namelen`. Query errors produce a diagnostic and status 1. A
non-positive result with no error is an indeterminate/no-limit result, so only
that length check is skipped.

| Requirement | Focused evidence |
| --- | --- |
| Initial and deeper query errors are diagnosed | `TestIssue741LimitsQueryErrorIsDiagnosed`, `TestIssue741DeepContainingDirectoryQueryError` |
| Indeterminate pathname/component limits do not become false failures | `TestIssue741IndeterminateLimitsSkipLengthChecks`, `TestIssue741DeepContainingDirectoryIndeterminateNameLimit` |
| Different containing directories may supply different limits | `TestPathchkDefaultUsesContainingDirectoryNameLimit` |
| Failure does not suppress later operands and final status is non-zero | `TestIssue741LimitsFailuresAggregateAcrossOperands`, `TestPathchkMultipleOperandsAggregateStatus` |
| Linux accepts a filesystem-valid non-UTF-8 component even in a UTF-8 locale | `TestIssue741LinuxAcceptsFilesystemValidNonUTF8Name` |
| Unsupported platforms fail closed instead of inventing limits | `TestIssue741PlatformWithoutLimitsQueryFailsClosed` |
| Terminator accounting, portable limits/characters, `-P`, searchability, and raw resolution order | Existing `pathchk_test.go` and `pathchk_unix_test.go` products |

## Byte-sequence validity decision

The default requirement concerns a byte sequence that is not valid in its
*containing directory*. It is not permission to reject every malformed UTF-8
argument merely because `LC_CTYPE` names a UTF-8 locale. Linux filesystems
such as ext4 accept arbitrary non-NUL, non-slash component bytes, and both GNU
and native implementations accept such names there. The implementation
therefore does not impose a locale-wide UTF-8 storage rule. Errors returned by
the actual pathname walk remain diagnostic failures.

There is no portable, side-effect-free syscall that asks whether a missing
component's byte sequence could be created in every mounted filesystem. A
filesystem-specific provider for missing-name syntax remains an explicit
non-Linux platform residual. This residual does not affect the Ubuntu/ext4
Profile C target, where arbitrary component bytes are valid. It must not be
misrepresented as locale validation.

Translated `LC_MESSAGES` catalogs and non-Linux/Darwin runtime providers also
remain outside this command-local closure. The shared Sprint 79 ledger stays
`partial` until those integration boundaries are dispositioned.

## Gates

- `go test -count=20 ./cmds/pathchk`
- `go test -race -count=5 ./cmds/pathchk`
- `go vet ./cmds/pathchk`
- repository cross-platform vet/build gates after integration on current main
