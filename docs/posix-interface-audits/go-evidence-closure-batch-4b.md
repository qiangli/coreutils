# POSIX Go evidence closure: wave 4B

This audit covers exactly `expr`, `fold`, `join`, `pathchk`, and `uniq`
against POSIX.1-2016 (Issue 7). POSIX is the normative source for the rows
below; GNU compatibility remains deferred except where existing non-POSIX
extensions already had documented behavior outside `POSIXLY_CORRECT`.

The five ledger rows are promoted only to `partial`. Focused command-package
tests now cover supported deterministic semantics, stream routing, operand
arity, selected diagnostics, output effects, and exit statuses. Verification is
not claimed because each command still has at least one exact residual.

## Verdicts and residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `expr` | [Issue 7 expr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html) | partial | Locale-directed string comparison, full locale-sensitive BRE collation/class behavior, translated diagnostics, and `NLSPATH` catalog lookup remain absent. GNU keyword functions remain extensions outside the POSIX base expression grammar. |
| `fold` | [Issue 7 fold](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html) | partial | The default and `-c` paths decode UTF-8 runes; non-UTF-8 public-locale charmap input can still be corrupted instead of round-tripping as locale characters. Translated diagnostics/catalog lookup remain absent. |
| `join` | [Issue 7 join](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html) | partial | Join-key comparison and sorted-order checks remain C-locale byte comparisons; required `LC_COLLATE` ordering and locale blank handling are not implemented. Translated diagnostics/catalog lookup remain absent. |
| `pathchk` | [Issue 7 pathchk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html) | partial | Default checks still use compile-time path and component limits instead of querying the underlying filesystem/containing directory, and the required default byte-sequence-validity check is absent. Translated diagnostics/catalog lookup remain absent. |
| `uniq` | [Issue 7 uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html) | partial | The C/POSIX byte-unit behavior of `-s` and the existing `-w` extension is now covered by a discriminating `LC_ALL=C`, `POSIXLY_CORRECT=1` fixture. This wave also makes POSIX mode reject zero for the positive-decimal `-f` and `-s` arguments while preserving GNU 9.11's accepted-zero behavior outside POSIX mode. The implementation remains deliberately byte-oriented under every environment: interpretation under non-C `LC_CTYPE`, locale-specific blank classes, translated diagnostics, and `NLSPATH` catalog lookup remain absent. |

## Evidence added

- `expr`: `cmds/expr/expr_test.go#TestExprPOSIXOperandsStdinAndDiagnostics`
  pins `--` operand handling, stdin non-use, missing-operand diagnostics, and
  status 2; existing POSIX arithmetic, boolean, match, result, and status tests
  remain referenced.
- `fold`: `cmds/fold/fold_test.go#TestFoldPOSIXOperandsAndErrors` pins
  file/stdin operand order and continuation after an unreadable operand.
- `join`:
  `cmds/join/join_test.go#TestJoinPOSIXOutputListAndUnpairableAggregation`,
  `#TestJoinPOSIXFieldSeparators`, and
  `#TestJoinPOSIXOperandArityAndStderr` pin `-a` aggregation, `-e`, `-o`, `-t`
  tab separation, one-stdin-only arity, and diagnostics on stderr.
- `pathchk`:
  `cmds/pathchk/pathchk_test.go#TestPathchkPosixNameLimitAndPortableCharacters`,
  `#TestPathchkPosixDoesNotRejectLeadingHyphen`,
  `#TestPathchkMultipleOperandsAggregateStatus`, and
  `#TestPathchkMissingOperandUsage` close the previous `-p` evidence gaps for
  name length, portable characters, the leading-hyphen rationale, aggregation,
  and arity.
- `uniq`: `cmds/uniq/uniq_test.go#TestUniqPOSIXCCharacterUnits` uses inputs
  that distinguish byte counting from unconditional UTF-8 decoding, and pins
  the C/POSIX byte-unit behavior of `-s`, the existing `-w` extension, and
  field-before-character skipping under explicit `LC_ALL=C` and
  `POSIXLY_CORRECT=1`; `#TestUniqPOSIXRequiresPositiveSkipArguments` pins the
  POSIX-only positive-decimal check and the preserved non-POSIX GNU behavior.

All 22 existing `shell_routing_evidence` references are preserved untouched.
