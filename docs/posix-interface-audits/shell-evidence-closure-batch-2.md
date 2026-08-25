# POSIX shell evidence closure: batch 2

This batch covers the shell-selected `cd`, `command`, `getopts`, `hash`, and
`pwd` interfaces against POSIX.1-2016 (Issue 7). The shell implementation is
owned by the sibling `sh` repository; Bashy's existing Profile B routing tests
independently prove that direct invocations select those shell built-ins.

All five ledger rows move from `unverified` to `partial`. Stable focused tests
now exercise their principal option, operand, state, stream, and status paths,
but the rows remain conservative: translated `LC_MESSAGES` diagnostics and
`NLSPATH` catalogs are absent, locale-sensitive diagnostics are not exercised,
and exceptional filesystem/process states are not exhaustive.

## Accepted semantic fixes

The reviewed shell commit fixes five behaviors, each differentially checked
against GNU Bash 5.3 in default and POSIX modes:

- `cd` and `pwd` now surface standard-output failures; a `cd` that must print
  its result still changes directory before returning failure, as Bash does.
- `cd` distinguishes an unset `HOME` (failure) from an explicitly empty
  `HOME` (Bash-compatible silent no-op).
- `hash` returns usage status 2 for an invalid option and silently skips shell
  functions, enabled built-ins, and slash-containing names unless the Bash
  `-p` extension explicitly installs an entry.
- multi-name Bash-extension forms of `command -v` and `command -V` now succeed
  when any requested name resolves, and fail only when none resolves.
- the empty-directory `cd` diagnostic now matches Bash 5.3 wording.

## Evidence and residuals

| Command | Issue 7 source | Stable semantic evidence | Exact residual before verification |
| --- | --- | --- | --- |
| `cd` | [Issue 7 cd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html) | `sh:interp/issue7_command_interface_test.go#TestCdIssue7Interface` | Translation/catalog support; exhaustive CDPATH entry ordering, permission, inaccessible-parent, and platform-specific filesystem failures. |
| `command` | [Issue 7 command](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html) | `sh:interp/issue7_command_interface_test.go#TestCommandIssue7Interface` | Translation/catalog support; exhaustive invoked-utility signal/redirection/126-status cases and implementation-defined `-V` descriptions. |
| `getopts` | [Issue 7 getopts](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html) | `sh:interp/issue7_command_interface_test.go#TestGetoptsIssue7Interface` | Translation/catalog and locale-sensitive diagnostics; exhaustive nested function/caller sharing and unspecified nonalphanumeric optstring characters. |
| `hash` | [Issue 7 hash](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html) | `sh:interp/issue7_command_interface_test.go#TestHashIssue7Interface` | Translation/catalog support and exhaustive PATH mutation/invalidation cases; `-p`, `-t`, and `-d` remain explicitly Bash extensions. |
| `pwd` | [Issue 7 pwd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html) | `sh:interp/issue7_command_interface_test.go#TestPwdIssue7Interface` | Translation/catalog support and injected getcwd/permission failures; ignored operands and the `-P` PWD update are Bash extensions. |

The sibling tests run the POSIX parser with both strict POSIX switches enabled
and a C locale. They passed ten repetitions, a focused race run, the existing
runner and broken-pipe tests, `go vet`, and source builds for Windows, Plan 9,
and JS/Wasm. An independent reviewer also reproduced every source-level delta
against the official Bash 5.3 container before approving integration.

The routing evidence remains the corresponding
`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRoute...` test for
each name. Semantic and routing evidence are separate fail-closed ledger
columns; neither is inferred from the other.
