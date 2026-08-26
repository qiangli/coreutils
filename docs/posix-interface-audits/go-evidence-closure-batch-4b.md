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
| `expr` | [Issue 7 expr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html) | partial | Locale wave A supplies invocation-owned carried-locale comparison and BRE classes, equivalence, and collation ranges. Issue 780 (`c418161`) adds required `\1`-`\9` back-references to the byte-locale path while preserving leftmost-longest selection and raw-byte capture offsets. The bounded locale/platform corpus remains the residual; GNU keyword functions remain extensions outside the POSIX base expression grammar. |
| `fold` | [Issue 7 fold](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html) | partial | Locale wave A proves character boundaries, widths, byte preservation, precedence, and fail-closed provider behavior for the carried C/POSIX, UTF-8, and ISO-8859-1 products. The locale corpus and Unicode width policy remain bounded. |
| `join` | [Issue 7 join](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html) | partial | Locale wave B routes join comparison and sorted-order checks through invocation-owned `LC_COLLATE`, stages output until compare/close success, and applies carried `LC_CTYPE` high-byte folding for `-i`. In the bounded C/German corpus the default blank class is space and tab; broader locale/platform products remain residual. |
| `pathchk` | [Issue 7 pathchk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html) | partial | Accepted Issue 744 source queries the containing filesystem at each depth, handles failed and indeterminate limits, aggregates diagnostics/status, and proves on Linux that filesystem-valid non-UTF-8 components are not rejected merely by a UTF-8 locale. Remaining: missing-name syntax on filesystems with additional encoding restrictions and non-Linux/Darwin runtime coverage. |
| `uniq` | [Issue 7 uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html) | partial | Issue 769 makes required `-f`/`-s` comparison character-oriented under carried multibyte and ISO-8859-1 `LC_CTYPE`, with locale blank handling, byte-exact output, precedence, and fail-closed provider tests; `-w` remains an explicitly byte-oriented GNU extension. The locale/provider corpus remains bounded. |

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
