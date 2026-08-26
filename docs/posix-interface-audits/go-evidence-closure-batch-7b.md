# POSIX Go evidence closure: wave 7B

Wave 7B audits the Go-owned `split`, `strings`, `stty`, `tabs`, `tput`, and
`tr` interfaces against POSIX.1-2016 Issue 7. All six rows move from
`unverified` to `partial`: required C/POSIX-locale behavior is covered, while
the residual locale and platform limits below remain explicit.

## Verdicts and residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `split` | [Issue 7 split](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html) | partial | The required line/byte forms, stdin and prefix defaults, two-letter POSIX suffix namespace and exhaustion, explicit suffix length, empty input, partial final line, unchanged output bytes, diagnostics, and status are covered. `POSIXLY_CORRECT` selects Issue 7's fixed default suffix length; the GNU auto-widening extension remains outside that mode. Multibyte filename `{NAME_MAX}` accounting and translated diagnostics remain absent. |
| `strings` | [Issue 7 strings](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html) | partial | Whole-file scanning, `-a`, positive `-n`, `-t d/o/x`, per-file byte offsets, regular-file operands, no-operand stdin, printable-string termination, read/write failures, diagnostics, and status are covered. C/POSIX, UTF-8, and the injected single-byte locale provider are character-granular; a complete system locale database and translated diagnostics remain absent. |
| `stty` | [Issue 7 stty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html) | partial | Default, `-a`, and restorable `-g` reports; the required control/input/output/local/combination operands; speeds; control characters; `min`/`time`; terminal-only stdin; atomic validation; output errors; diagnostics; and status are covered by real PTYs. Windows has no termios implementation, and translated diagnostics remain absent. |
| `tabs` | [Issue 7 tabs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tabs.html) | partial | `-T`, the default and single-digit repetitive forms (including `-0`), every XSI preset, explicit comma/blank/increment lists, strict ascending validation, terminal capability resolution, exact emitted bytes, unavailable-terminal errors, write errors, and status are covered. Coverage uses deterministic terminfo fixtures; exhaustive host terminal databases and translated diagnostics remain absent. |
| `tput` | [Issue 7 tput](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tput.html) | partial | `-T`, TERM/default resolution, ordered `clear`/`init`/`reset` operand processing, continuation past unavailable operations, exact sequences, output errors, and the Issue 7 status partition (0, 2, 3, 4, and greater than 4) are covered. Coverage uses deterministic terminfo fixtures; exhaustive host terminal databases and translated diagnostics remain absent. |
| `tr` | [Issue 7 tr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html) | partial | Issues 769 and 781 cover all four forms plus carried-locale character boundaries, classes, ranges, equivalence, repetitions, binary-value `-c`, LC_CTYPE-character `-C` ordered by LC_COLLATE, post-transform squeezing, exact invalid/raw bytes, I/O failures, and provider preflight/lifecycle. The final `-C` repair (`9e53b12`) proves that every carried LC_CTYPE character, including NUL, remains in the domain even when LC_COLLATE cannot expose it as a named collating element. The locale/provider corpus remains bounded rather than a general installed-locale implementation. |

## Evidence added or closed

- `split`: `cmds/split/split_test.go#TestSplitPOSIXDefaultSuffixExhaustion`
  proves that POSIX mode uses exactly `xaa` through `xzz`, then fails without
  inventing a wider suffix.
- `strings`: `cmds/strings/strings_test.go#TestStringsIOErrorsAreFailures`
  pins input and output failure diagnostics and non-zero status.
- `stty`: `cmds/stty/stty_posix_test.go#TestSttyRequiredReportsPropagateWriteErrors`
  drives default, `-a`, and `-g` reports from a real PTY into a failing writer.
- `tabs`: `cmds/tabs/tabs_test.go#TestTabsOutputWriteErrorIsReported` closes
  the terminal-sequence output-error path.
- `tput`: existing `cmds/tput/tput_test.go#TestPOSIXOperationSequence` and
  `#TestPOSIXOperationSequenceAvailabilityAndErrors` exactly cover ordered
  required operands, unavailable operations, invalid later operands, and
  write failure without conflating GNU capability parameters with POSIX mode.
- `tr`: `cmds/tr/tr_test.go#TestTrInputReadErrorIsReported` proves that a
  non-EOF stdin failure flushes completed transformations, diagnoses the read
  failure, and exits non-zero.

Focused count-10 and race tests cover all six packages. Focused host vet and
Linux, Darwin, and Windows cross-build/vet gates are recorded with the batch
commit; shared aggregate manifests are intentionally left for root to
regenerate after integration.

## Proposed aggregate reconciliation

Root should move the `split`, `strings`, `stty`, `tabs`, `tput`, and `tr` rows
in `docs/posix-required-command-interfaces.tsv` from `unverified` to `partial`,
copying the normative behavior and exact residuals above. Suggested Go
evidence IDs are the tests listed in this note plus the existing core suites:
`TestSplitLines`, `TestSplitBytes`, `TestSplitOperandsAndPrefix`, `TestStrings`,
`TestStringsFiles`, `TestSttyRowsColsRejectsOverflow`, the PTY subtests under
`TestSttyRowsColsRejectsOverflow`, `TestPresetColumnsMatchPOSIX`,
`TestRepetitiveSpec`, `TestExplicitList`, `TestIncrementForm`,
`TestTerminalTypeResolution`, `TestExitStatuses`, `TestInitAndReset`,
`TestTrTranslate`, `TestTrDeleteSqueeze`, `TestTrClasses`,
`TestCTypeProviderBacked`, and `TestCTypeCaseTranslation`. No shared manifest
is changed by this batch.
