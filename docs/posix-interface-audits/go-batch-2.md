**POSIX Interface Audit — Go Batch 2: csplit cut date dd df diff dirname du env expand expr file find**

Audits the 13 effective Go-owned POSIX commands not covered by go-batch-1.
Evidence chain, per command: the authoritative Open Group Base Specifications
Issue 7, 2016 Edition page (all 13 fetched fresh from
`https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/<cmd>.html`
on 2026-08-24), the Go implementation in `cmds/<cmd>/`, the in-tree test
suite (cited as `file#TestID`), and live multicall probes performed for the
issue-27 draft (go1.26.5 darwin/arm64, `LC_ALL=C`); load-bearing claims were
re-verified against this worktree's source and tests before writing. That
draft is used as evidence only; this document supersedes it.

Classification vocabulary: **verified** (spec element exists, behaves per
spec, tested); **implementation_gap** (spec element missing or behaving
differently — loud rc≥2 error where applicable); **evidence_gap**
(implemented, but untested or under-tested in-tree).

**Locale ruling (applies to all 13 commands).** Every one of the 13 spec
pages lists an ENVIRONMENT section naming `LANG`, `LC_ALL`, `LC_CTYPE`, and
`LC_MESSAGES` (plus `LC_COLLATE` for csplit, expr, find; `LC_TIME` and `TZ`
for date and diff). This implementation is locale-invariant (fixed C-locale
semantics, repo-wide agent contract). Locale invariance is **not** treated
as "verified by policy" here: each spec-listed locale effect that the
implementation does not implement is an **implementation_gap**, and any
locale-handling path that exists but lacks non-C test coverage is an
**evidence_gap**. `TZ` is the one locale-adjacent variable that is
implemented and tested (date, diff headers). `POSIXLY_CORRECT` (GNU's
POSIX-mode switch, e.g. 512-byte df/du blocks) is absent from all 13
packages — a GNU-mode note, not a POSIX element, and not counted as a POSIX
gap below.

This file is audit-local. It promotes nothing: `docs/posix-interfaces.md`
and `docs/posix-vsc-pcts-status.md` evidence states are unchanged.

**Confirmed gaps, ranked by VSC-PCTS exposure**

| # | Command | Gap | Class |
|---|---------|-----|-------|
| 1 | df | Default units must be 512-byte blocks; implementation uses 1024 (`cmds/df/df.go:87`) | implementation_gap |
| 2 | df | `-P` without `-k` must use 512-byte units, header `512-blocks`, and the header word `Capacity`; implementation prints `1024-blocks`/`Use%` unconditionally (`cmds/df/df.go:185,216`) | implementation_gap |
| 3 | df | XSI `-t` must print a totals row taking no argument; `-t` is bound to GNU `--type` (type filter requiring an argument, `cmds/df/df.go:70`); a totals row exists only under GNU `--total` | implementation_gap (flag repurposed) |
| 4 | du | Default units must be 512-byte blocks; implementation uses 1024 (`cmds/du/du.go:215`) and the test suite locks the deviation (`cmds/du/du_test.go#TestDefaultUnitIs1K`) | implementation_gap |
| 5 | du | STDOUT separator must be a single `<space>` (`"%d %s\n"`); implementation emits TAB (`cmds/du/du.go:434`) | implementation_gap |
| 6 | dd | XSI `conv=ascii`, `conv=ebcdic`, `conv=ibm` not implemented (no code path; rejected loudly) | implementation_gap (documented subset) |
| 7 | date | XSI set-date operand `mmddhhmm[[cc]yy]` not supported; rejected loudly listing supported forms (`cmds/date/date.go:111,138`) | implementation_gap (documented subset) |
| 8 | file | Required options `-d`, `-i`, `-M` unbound (loud usage error); `-m` is bound; default does not dereference symlinks (POSIX default resolves, `-h` opts out); unreadable operand leaves exit status 0 (POSIX: 0 only on successful completion of all operands); required type strings `commands text`/`c program text` not produced (GNU wording instead) | implementation_gap |
| 9 | all 13 | Spec-listed `LANG`/`LC_*` effects unimplemented (see locale ruling) | implementation_gap / evidence_gap per command |

Correction versus the issue-27 draft: `dd` `conv=block`, `conv=unblock`, and
`cbs=` **are** implemented in this worktree (`cmds/dd/dd.go:44-45`;
`cmds/dd/dd_test.go#TestDdBlockAndUnblockConversions`,
`#TestDdBlockSyncUsesSpacePadding`) — the draft's claim that this machinery
waits on the missing EBCDIC conversions is wrong; only
`conv=ascii|ebcdic|ibm` remain gaps.

## csplit

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/csplit.html>

Synopsis `csplit [-ks] [-f prefix] [-n number] file arg...`; operands are
line numbers, `/rexp/[offset]`, `%rexp%[offset]`, with `{number}`/`{*}`
repetition; `file` may be `-`. STDOUT: unless `-s`, one `%ld\n` size per
piece in order. Consequences of errors (spec): created files removed on
error unless `-k`. Exit: 0, >0 error. All verified:
`cmds/csplit/csplit.go`; `cmds/csplit/csplit_test.go#TestCsplitLineNumber`,
`#TestCsplitRegexAndPrefix`, `#TestCsplitRegexOffsets`,
`#TestCsplitRepeatedRegexAdvances`, `#TestCsplitRepeatToEOF`,
`#TestCsplitZeroRepeatIsNoop`, `#TestCsplitSuffixSuppressRepeatAndElideEmpty`,
`#TestCsplitLineNumberRepeatToEOFCleansUp`, `#TestCsplitLineNumberRepeatOutOfRange`,
`#TestCsplitPatternsAreBRE`, `#TestCsplitErrors`. Environment: LANG,
LC_ALL, LC_COLLATE, LC_CTYPE, LC_MESSAGES. Gaps: LC_COLLATE-directed BRE
collation unimplemented (byte/C semantics) — implementation_gap; escaped
delimiters (`\/rexp\/`) implemented but untested — evidence_gap; message
translation — implementation_gap. Verdict: **verified** except the
locale/evidence gaps above.

## cut

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html>

Three mutually exclusive forms (`-b list [-n]`, `-c list`, `-f list [-d
delim] [-s]`); list grammar with ascending ranges, no `0`; `-` operand is
stdin; exit 0/>0. All verified including `-n` (the generated ledger's doubt
is resolved): `cmds/cut/cut.go`; `cmds/cut/cut_test.go#TestCutFields`,
`#TestCutBytesAndChars`, `#TestCutBytesNoSplit`, `#TestCutFiles`,
`#TestCutUsageErrors`. Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES.
Gap: LC_CTYPE-directed character classification unimplemented
(fixed UTF-8/byte handling) — implementation_gap; message translation —
implementation_gap. Verdict: **verified** modulo locale gaps.

## date

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html>

Base synopsis `date [-u] [+format]` verified: directive set, field widths,
`%n`/`%t`/`%%`; unknown directives print literally (spec: unspecified).
Exit 0/>0. `TZ` implemented and tested (`cmds/date/date_test.go#TestDateTZ`).
Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES, LC_TIME, TZ. Gaps: XSI
set-date operand `mmddhhmm[[cc]yy]` rejected loudly naming supported forms
(RFC 3339, `@EPOCH`, `YYYY-MM-DD [HH:MM[:SS]]`) — implementation_gap
(documented subset); LC_TIME precedence logic is exercised
(`cmds/date/date_test.go#TestDateLCTimePrecedence`) but non-C LC_TIME
rendering has no test coverage — evidence_gap; LC_MESSAGES/LC_CTYPE —
implementation_gap. Evidence: `cmds/date/date.go`; also
`#TestDateFormats`, `#TestDateErrors`, `#TestDateInvalidUsageDiagnostics`.
Verdict: base **verified**; XSI set-date gap stands.

## dd

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dd.html>

No options, only `name=value` operands (`if= of= ibs= obs= bs= cbs= skip=
seek= count= conv=`), size suffixes `b k K m M w x` — verified, including
`conv=block`/`unblock`/`cbs=` pairing, `sync`, `ucase`/`lcase`, `swab`,
`noerror`, `notrunc`, `iflag=fullblock`, `status=none`. Stdin/stdout used
when `if=`/`of=` absent. Exit 0/>0 (usage errors rc=2). Records diagnostic
emits a GNU-style `N bytes copied` trailer rather than the spec's example
table wording — note only (wording not normative; totals present).
Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES. Gaps: XSI
`conv=ascii|ebcdic|ibm` not implemented, loud rc=2 — implementation_gap;
locale effects — implementation_gap. Evidence: `cmds/dd/dd.go`;
`cmds/dd/dd_test.go#TestDdStdinStdout`, `#TestDdOperandSizeSyntax`,
`#TestDdSkipSeekNotrunc`, `#TestDdNoerrorSyncNotruncKeepsExistingTail`,
`#TestDdCaseConversionsAreSingleByteAndReblockBs`,
`#TestDdSwabPreservesOddByteInEachInputRecord`,
`#TestDdBlockAndUnblockConversions`,
`#TestDdSyncPadsEachShortInputRecordBeforeReblocking`,
`#TestDdCopiesFileWithStatusNone`, `#TestDdErrors`. Verdict: operand
grammar, sizes, stdio, exit statuses **verified**; EBCDIC conversions gap
stands.

## df

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/df.html>

Synopsis (XSI utility) `df [-k] [-P|-t] [file...]`. `-k` selects 1024-byte
units; `-P` the portable one-line-per-filesystem format with header
`Filesystem 512-blocks Used Available Capacity Mounted on` (`1024-blocks`
only with `-k`); XSI `-t` prints a totals row, no option-argument. Exit
0/>0. Implementation: `-k`/`-P` formats exist and are tested, but the unit
default is 1024 (`cmds/df/df.go:87` `scaleMode{blockSize: 1024, header:
"1K-blocks"}`), `-P` prints `1024-blocks` (`df.go:185`) and the GNU header
word `Use%` (`df.go:216`) instead of `Capacity`, and `-t` is bound to GNU
`--type` filesystem filtering requiring an argument (`df.go:70`), so
`df -t` alone is a loud rc=2 flag error, never a totals row; a totals row
exists only under GNU `--total` (`df.go:473`). Environment: LANG, LC_ALL,
LC_CTYPE, LC_MESSAGES — implementation_gap (locale effects
unimplemented). Evidence: `cmds/df/df_test.go#TestDefaultListing`,
`#TestKFlagSameUnits`, `#TestPortablePrintTypeAndInodesHeaders`,
`#TestTypeFiltersAndTotal`, `#TestNonexistentOperand`, `#TestUsePct`,
`#TestUnknownFlag`. Verdict: three implementation_gaps as ranked (units
default, `-P` 512/Capacity wording, `-t` repurposed) plus locale; the
`-t` repurposing is the most severe finding in this batch because an
upstream spelling carries a different documented meaning.

## diff

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/diff.html>

Synopsis `diff [-c|-e|-f|-u] [-br] file1 file2` (options XSI) plus `-C n`/
`-U n`; `-` operand is stdin (exactly one); directories recurse with
`Only in`/`Common subdirectories`; exit 0/1/>1. All verified: normal,
context, unified, and ed formats; `-b` blank-run equality; recursion.
Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES, LC_TIME, TZ. `TZ` is
honored in `-c`/`-u` headers and tested
(`cmds/diff/diff_test.go#TestHeadersHonorEnvTZMinuteOffsets`); LC_TIME
header rendering is fixed C-locale — implementation_gap; LC_CTYPE/
LC_MESSAGES — implementation_gap. Evidence: `cmds/diff/diff.go`;
`cmds/diff/diff_test.go#TestNormalFormat`, `#TestUnifiedGolden`,
`#TestContextGolden`, `#TestEdFormats`, `#TestRecursiveGolden`,
`#TestBrief`, `#TestIgnoreFlags`, `#TestStdin`, `#TestErrors`. The
generated ledger's warning that attached `-C=<n>`/`-U=<n>` forms are
missing was re-checked and is a false positive (both spellings parse;
tests cover `-C`/`-U` with arguments). Verdict: **verified** modulo
locale gaps.

## dirname

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html>

Synopsis `dirname string`: prefix up to (not including) the last `/`, else
`.`; all-slash strings yield `/`; exit 0/>0. Verified:
`cmds/dirname/dirname.go`; `cmds/dirname/dirname_test.go#TestDirname`,
`#TestDirnameErrors`. Multiple operands and `-z` are GNU extensions under
upstream spellings — noted, out of POSIX scope. Environment: LANG,
LC_ALL, LC_CTYPE, LC_MESSAGES — implementation_gap (locale effects
unimplemented). Verdict: POSIX-required behavior **verified**.

## du

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/du.html>

Synopsis (XSI) `du [-a|-s] [-kx] [-H|-L] [file...]`; `-a`/`-s` exclusive;
STDOUT format is the block count followed by a single `<space>` and the
pathname; exit 0/>0. The spec's RATIONALE states 512-byte units are
deliberate for compatibility with `ls`. Options, exclusivity, and symlink
modes verified (`cmds/du/du_test.go#TestAll`, `#TestSummarize`,
`#TestSymlinkDereferenceModes`, `#TestConflictingFlags`,
`#TestFileOperand`). Two implementation_gaps: default units are 1024
(`cmds/du/du.go:215` `block := int64(1024)`; deviation locked by
`cmds/du/du_test.go#TestDefaultUnitIs1K`) and the separator is TAB
(`cmds/du/du.go:434` `"%s\t%s%s"`). Environment: LANG, LC_ALL, LC_CTYPE,
LC_MESSAGES — implementation_gap. Verdict: options **verified**; units
and separator gaps stand.

## env

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html>

Synopsis `env [-i] [name=value]... [utility [argument...]]`; `-` as first
operand is unspecified and treated as `-i` (GNU-compatible, acceptable);
PATH from the modified environment locates `utility` (verified); exit
status passes through from `utility`, else 0/1-125/126 not
executable/127 not found — all verified. env is one of the repo's
documented spawn-exceptions (the operand IS the utility). Evidence:
`cmds/env/env.go`; `cmds/env/env_test.go#TestEnv`,
`#TestEnvRunsCommandWithModifiedEnvironment`,
`#TestEnvCommandStdioAndExitStatus`,
`#TestEnvCommandNotFoundAndNotExecutable`, `#TestEnvErrors`;
`cmds/env/env_exec_test.go#TestEnvExecPassesArgvVerbatimWithoutShell`,
`#TestEnvExecEmptyPathSearchesWorkingDirectoryOnly`. Environment
(affecting env's own diagnostics): LANG, LC_ALL, LC_CTYPE, LC_MESSAGES —
implementation_gap. Verdict: **verified**.

## expand

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html>

Synopsis `expand [-t tablist] [file...]`; tablist is a single positive
decimal or comma/blank-separated strictly ascending list; a tab at or
beyond the last stop expands to a single space; `-` operand is stdin;
exit 0/>0 with a diagnostic on operand access failure. All verified:
`cmds/expand/expand.go`; `cmds/expand/expand_test.go#TestExpandDefaultTabsFromStdin`,
`#TestExpandCustomTabsAndFile`, `#TestExpandTabListIncrement`,
`#TestExpandBlankSeparatedTabList`,
`#TestExpandTabsBeyondLastStopBecomeSingleSpaces`, `#TestExpandRejectsBadTabs`.
Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES. Gap: LC_CTYPE-directed
blank/column classification unimplemented (fixed UTF-8/byte logic —
`#TestExpandWideRuneCountsDisplayColumns`,
`#TestExpandCombiningMarkIsZeroWidth` cover the C/UTF-8 paths only) —
implementation_gap; message translation — implementation_gap. Verdict:
**verified** modulo locale gaps.

## expr

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html>

Operators `| & = > >= < <= != + - * / % :` with precedence and parens;
anchored BRE matching for `:`; decimal output with leading zeroes
removed; exit 0 result non-null/non-zero, 1 null/zero, 2 invalid
expression, >2 error (division by zero → 3). All verified:
`cmds/expr/expr.go`; `cmds/expr/expr_test.go#TestExprArithmetic`,
`#TestExprMatch`, `#TestExprPOSIXArithmeticAndComparison`,
`#TestExprPOSIXBooleanAndExitStatus`, `#TestExprPOSIXMatchAndStringFunctions`.
Environment: LANG, LC_ALL, LC_COLLATE, LC_CTYPE, LC_MESSAGES. Gaps:
LC_COLLATE-directed string comparison unimplemented (byte order) —
implementation_gap; locale-directed BRE collation — implementation_gap;
message translation — implementation_gap. GNU keyword tokens (`index
length match substr`) are extensions — noted. Verdict: **verified**
modulo locale gaps.

## file

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/file.html>

Synopsis `file [-dh] [-M file] [-m file] file...` and `file [-i [-h]
file...]`; `-d` default position-sensitive tests, `-h` no dereference,
`-i` minimal identification, `-M file`/`-m file` magic files; `-`
operand is stdin (verified); output `<pathname>: <type>` with required
strings `cannot open`, and descriptions containing `commands text`
(shell scripts) and `c program text` (C sources); exit 0 on successful
completion of all operands, >0 on error. Implementation: `-h` bound and
tested; `-m` bound as magic-file augmentation
(`cmds/file/file.go:29`, `cmds/file/file_test.go#TestAdditionalMagicFileFallsBackToBuiltins`);
`-d`, `-i`, `-M` are unbound → loud usage error — implementation_gap;
default is no-dereference (`cmds/file/file.go:27` flags help text
"(default)", `file.go:65` `Lstat`) where POSIX resolves symlinks by
default — implementation_gap; unreadable operand prints `cannot open`
but exits 0 — implementation_gap; type strings use GNU wording
(`script, text executable`, `C source`) not the required containment —
implementation_gap. Environment: LANG, LC_ALL, LC_CTYPE, LC_MESSAGES —
implementation_gap. Evidence: `cmds/file/file.go`;
`cmds/file/file_test.go#TestTextEmptyDataAndStdin`,
`#TestPortableSignatures`, `#TestDirectorySymlinkAndFollow`,
`#TestNoDereferenceShortOptionAndTrailingSlash`,
`#TestErrorsAndStrictFlags`. Verdict: `-h`/`-m` **verified**; the four
option/operand/status/string gaps stand. PCTS exposure is tempered:
GNU `file` deviates from the same POSIX strings and the file tsets are
known-lenient.

## find

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html>

Synopsis `find [-H|-L] path... [operand_expression...]`; default `-P`
never dereferences (verified); primaries including `-name -path -lname
-type -perm -user -group -nouser -nogroup -size -links -inum -newer
-atime -ctime -mtime -depth -mount -xdev -local -prune -follow -exec
-ok -print -print0`, operators `-a -o ! ( )`; `{} +` batching with any
failing invocation forcing nonzero exit (verified), `-ok` reads a `y`
response; exit 0 all paths traversed, >0 otherwise. Evidence:
`cmds/find/find.go`; `cmds/find/find_test.go#TestFindTests`,
`#TestFindDefaultPath`, `#TestFindPrint0`, `#TestFindMtimeAndNewer`,
`#TestFindTypeSymlink`, `#TestFindDepthOrder`, `#TestFindXdev`,
`#TestFindPerm`, `#TestFindUserGroup`, `#TestFindExecSemicolon`,
`#TestFindExecPlus`, `#TestFindExecStatusSemantics`, `#TestFindOk`,
`#TestFindErrors`; `cmds/find/conformance_test.go`. Note (not a gap):
`-exec ... \;` child failure does not affect exit status — exactly what
POSIX specifies (only `{} +` mandates nonzero); GNU additionally
returns 1, so this is a GNU deviation, not a POSIX one. Environment:
LANG, LC_ALL, LC_COLLATE, LC_CTYPE, LC_MESSAGES. Gaps:
LC_COLLATE-directed pattern collation unimplemented — C-locale
invariance is deliberate and tested
(`cmds/find/find_test.go#TestFindPatternsAreCLocaleAndLocaleInvariant`)
but the spec-listed locale effect is absent — implementation_gap;
message translation — implementation_gap. Verdict: **verified** modulo
locale gaps.
