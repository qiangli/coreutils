# POSIX Issue 7 interface audit: `cksum`, `cmp`, and `head`

This audit covers only the Go-owned `cksum`, `cmp`, and `head` commands. The
authoritative interfaces are the Issue 7 pages linked from their rows in
`docs/posix-required-command-interfaces.tsv`. GNU extensions were not used as
positive evidence.

## Disposition

| Command | Observable Issue 7 evidence | Residual; row remains partial |
| --- | --- | --- |
| `cksum` | No-option CRC and byte-count output, no-operand stdin form, named-file output, ordered continuation after an open failure, permitted `-` stdin treatment, input-read failure, stdout-write failure, and nonzero aggregate status. | `LC_MESSAGES` catalog lookup/translated diagnostics and real device-file behavior are not evidenced. The non-POSIX digest/check surface is deliberately not evidence. |
| `cmp` | Exact `[-l|-s] file1 file2` grammar under `POSIXLY_CORRECT`, `-` stdin routing, identical/different/EOF statuses, exact default and `-l` POSIX output, `-s`, input-read and normal-output write errors, and undefined repeated-stdin/same-special-source cases rejected deterministically. | Locale-specific message translation and live FIFO/block/character-device integration remain unverified. GNU omitted-file2, skips, and extra options are retained outside POSIX mode but are not evidence. |
| `head` | Required `-n number`, default ten-line behavior, shorter-file behavior, ordered multiple operands and headers, continuation after open failure, permitted `-` stdin treatment, input-read failure, stdout-write failure, and nonzero error status. | The application constraint that `number` is a positive decimal integer, locale message catalogs, and real text-file/device integration beyond the injected stream errors remain residual. GNU byte/count/header extensions are not evidence. |

## Confirmed correction

`cmp` previously accepted GNU's omitted second file and trailing skip operands
even with `POSIXLY_CORRECT` present. That left the Issue 7 required operand
grammar unenforced. POSIX mode now rejects fewer or more than two operands
before opening either input; GNU operand extensions continue to work outside
that mode. `cmp` also now treats a failed normal-output write as an error,
diagnoses it on stderr, and returns status 2.

## Focused regressions

- `cmds/cksum/cksum_test.go#TestCKSumStandardInputOperandAndReadError`
- `cmds/cksum/cksum_test.go#TestCKSumReportsStandardOutputWriteError`
- `cmds/cmp/cmp_posix_test.go#TestCmpPOSIXOperandGrammarAndOutputErrors`
- `cmds/cmp/cmp_posix_test.go#TestCmpVerbosePOSIXModeFormat`
- `cmds/cmp/cmp_posix_test.go#TestCmpVerboseEOFDiagnostic`
- `cmds/head/head_test.go#TestHeadStandardInputOperandAndReadError`
- `cmds/head/head_test.go#TestHeadErrors`
- `cmds/head/head_test.go#TestHeadWriteError`

The `-` treatment in `cksum` and `head` is an Issue 7-permitted
implementation choice, not a GNU-derived requirement; the regressions make
that declared behavior observable. All three ledger rows therefore remain
`partial`, with the exact residuals above rather than a promotion based solely
on unit-test count.
