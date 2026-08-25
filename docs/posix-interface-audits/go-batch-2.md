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

**Locale classification.** Every command is classified separately from its
source and tests. The spec pages name `LANG`, `LC_ALL`, `LC_CTYPE`, and
`LC_MESSAGES`; command-specific additions include `LC_COLLATE`, `LC_TIME`,
and `TZ`. They also name XSI `NLSPATH` for locating message catalogs. Fixed
C-locale behavior is not "verified by policy": a required locale effect that
is absent is an **implementation_gap**, while an implemented effect without a
test is an **evidence_gap**. Conversely, this worktree really does implement
and test non-C behavior for date (`LC_TIME`) and find (`LC_COLLATE`,
`LC_CTYPE`, and the `LC_MESSAGES` affirmative expression); those paths are
not reported as gaps. No command in this batch implements translated
diagnostic catalogs/`NLSPATH`, so that XSI surface remains an implementation
gap throughout. `POSIXLY_CORRECT` is GNU's POSIX-mode switch, not a POSIX
utility interface element; its absence is not itself counted as a gap.

This file is audit-local. It promotes nothing: `docs/posix-interfaces.md`
and `docs/posix-vsc-pcts-status.md` evidence states are unchanged.

**Confirmed gaps, ranked by VSC-PCTS exposure**

| # | Command | Gap | Class |
|---|---------|-----|-------|
| 1 | df | Default units must be 512-byte blocks; implementation uses 1024 (`cmds/df/df.go:87`) | implementation_gap |
| 2 | df | `-P` without `-k` must use 512-byte units, header `512-blocks`, and the header word `Capacity`; implementation prints `1024-blocks`/`Use%` unconditionally (`cmds/df/df.go:185,216`) | implementation_gap |
| 3 | df | XSI `-t` takes no argument and must include total allocated-space figures in each applicable filesystem record; implementation binds it to GNU `--type`, requiring a filesystem-type argument (`cmds/df/df.go:70`) | implementation_gap (flag repurposed) |
| 4 | du | Default units must be 512-byte blocks; implementation uses 1024 (`cmds/du/du.go:215`) and the test suite locks the deviation (`cmds/du/du_test.go#TestDefaultUnitIs1K`) | implementation_gap |
| 5 | du | STDOUT separator must be a single `<space>` (`"%d %s\n"`); implementation emits TAB (`cmds/du/du.go:434`) | implementation_gap |
| 6 | dd | SIGINT has no special handling; POSIX requires current status followed by termination as SIGINT | implementation_gap |
| 7 | dd | XSI `conv=ascii`, `conv=ebcdic`, `conv=ibm` not implemented (no code path; rejected loudly) | implementation_gap (documented subset) |
| 8 | dd | Default stderr adds a non-POSIX `N bytes copied` transfer line after the required records-in/out lines | implementation_gap |
| 9 | date | XSI set-date operand `mmddhhmm[[cc]yy]` not supported; rejected loudly listing supported forms (`cmds/date/date.go:111,138`) | implementation_gap (documented subset) |
| 10 | file | Required options `-d`, `-i`, `-M` unbound; `-m` implements only a narrow string-test subset of the required magic grammar; default does not dereference symlinks; required `commands text`/`c program text` strings are absent | implementation_gap |
| 11 | applicable commands | Required category effects and XSI message-catalog/`NLSPATH` behavior not implemented, except the explicitly verified date/find/TZ paths below | implementation_gap / evidence_gap per command |

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
error unless `-k`. Exit: 0, >0 error. The cited tests verify the core
line/regex/repetition/output/cleanup cases:
`cmds/csplit/csplit.go`; `cmds/csplit/csplit_test.go#TestCsplitLineNumber`,
`#TestCsplitRegexAndPrefix`, `#TestCsplitRegexOffsets`,
`#TestCsplitRepeatedRegexAdvances`, `#TestCsplitRepeatToEOF`,
`#TestCsplitZeroRepeatIsNoop`, `#TestCsplitSuffixSuppressRepeatAndElideEmpty`,
`#TestCsplitLineNumberRepeatToEOFCleansUp`, `#TestCsplitLineNumberRepeatOutOfRange`,
`#TestCsplitPatternsAreBRE`, `#TestCsplitErrors`. Environment: `LANG`,
`LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`.
Gaps: locale-directed BRE collation and character handling are unimplemented
(byte/C semantics) — implementation_gap; escaped delimiters (`\/rexp\/`) are
implemented but untested — evidence_gap; translated messages/catalog lookup
are absent — implementation_gap. Status: **partial**; the cited non-locale
interface is verified and the listed gaps remain.

## cut

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html>

Three mutually exclusive forms (`-b list [-n]`, `-c list`, `-f list [-d
delim] [-s]`); list grammar with ascending ranges, no `0`; `-` operand is
stdin; exit 0/>0. These elements are verified, including `-n` (the generated
ledger's doubt is resolved): `cmds/cut/cut.go`;
`cmds/cut/cut_test.go#TestCutFields`,
`#TestCutBytesAndChars`, `#TestCutBytesNoSplit`, `#TestCutFiles`,
`#TestCutUsageErrors`. Environment: `LANG`, `LC_ALL`, `LC_CTYPE`,
`LC_MESSAGES`, and XSI `NLSPATH`. Gaps: locale-directed character boundaries
are unimplemented (fixed UTF-8/byte handling), and translated
messages/catalog lookup are absent — implementation_gap. Status: **partial**;
the cited option/list/stdio behavior is verified and the locale gaps remain.

## date

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html>

Base synopsis `date [-u] [+format]` verified: directive set, field widths,
`%n`/`%t`/`%%`; unknown directives print literally (spec: unspecified).
Exit 0/>0. `TZ` implemented and tested (`cmds/date/date_test.go#TestDateTZ`).
Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `LC_TIME`, `TZ`,
and XSI `NLSPATH`. Gap: XSI set-date operand `mmddhhmm[[cc]yy]` is rejected
loudly while naming supported forms
(RFC 3339, `@EPOCH`, `YYYY-MM-DD [HH:MM[:SS]]`) — implementation_gap
(documented subset). `LC_TIME` precedence and non-C rendering are implemented
and tested for de_DE UTF-8 and ISO-8859-1, including `LC_ALL`/`LC_TIME`/`LANG`
precedence (`cmds/date/date_test.go#TestDateLCTimePrecedence`); `TZ` is likewise
verified. Locale-sensitive diagnostics/catalog lookup and other unsupported
locale effects remain implementation gaps. Evidence: `cmds/date/date.go`; also
`#TestDateFormats`, `#TestDateErrors`, `#TestDateInvalidUsageDiagnostics`.
Status: **partial**; display mode and the cited locale/TZ paths are verified,
while set-date and message-catalog gaps remain.

## dd

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dd.html>

POSIX has no options, only `name=value` operands (`if= of= ibs= obs= bs=
cbs= skip= seek= count= conv=`). Its size expression is a positive decimal,
optionally followed by lowercase `k` or `b`, or a product of those factors
separated by `x`. That grammar and the listed operands are verified, including
`conv=block`/`unblock` with `cbs=`, `sync`, `ucase`/`lcase`, `swab`,
`noerror`, and `notrunc`; stdin/stdout are used when `if=`/`of=` are absent.
Uppercase `K`, `M`, `m`, `w`, IEC/SI suffixes, `iflag=fullblock`, and
`status=none|noxfer` are GNU extensions, not POSIX evidence.

Exit 0/>0 and the required records-in/records-out lines are implemented and
tested. XSI `conv=ascii|ebcdic|ibm` is implemented. SIGINT emits current status
once and preserves signal termination at the standalone boundary; descriptor
waits remain cancellable under input and output backpressure. The remaining
status-format gap is that default stderr appends a non-POSIX `N bytes copied`
transfer line after the required status lines. Interruptible pathname-based
FIFO input is exact on Linux; Darwin refuses it immediately with an explicit
unsupported diagnostic because XNU cannot reveal a writer transition lost
before the first read.
Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`.
Locale-directed `lcase`/`ucase` and translated messages/catalog lookup are
unimplemented — implementation_gap. Evidence: `cmds/dd/dd.go`;
`cmds/dd/dd_test.go#TestDdStdinStdout`, `#TestDdOperandSizeSyntax`,
`#TestDdSkipSeekNotrunc`, `#TestDdNoerrorSyncNotruncKeepsExistingTail`,
`#TestDdCaseConversionsAreSingleByteAndReblockBs`,
`#TestDdSwabPreservesOddByteInEachInputRecord`,
`#TestDdBlockAndUnblockConversions`,
`#TestDdSyncPadsEachShortInputRecordBeforeReblocking`,
`#TestDdCopiesFileWithStatusNone`, `#TestDdErrors`,
`#TestDdSyncWithBsCountsPaddedOutputRecord`, and the signal/FIFO tests in
`cmds/dd/dd_signal_unix_test.go`. Status: **partial**; the cited
operand/copy/status/SIGINT machinery is verified, while the default transfer
line, Darwin pathname-FIFO boundary, and locale gaps remain.

## df

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/df.html>

Synopsis (XSI utility) `df [-k] [-P|-t] [file...]`. `-k` selects 1024-byte
units; `-P` the portable one-line-per-filesystem format with header
`Filesystem 512-blocks Used Available Capacity Mounted on` (`1024-blocks`
only with `-k`); XSI `-t` takes no option-argument and includes total
allocated-space figures in the applicable filesystem records. Exit
0/>0. Implementation: `-k`/`-P` formats exist and are tested, but the unit
default is 1024 (`cmds/df/df.go:87` `scaleMode{blockSize: 1024, header:
"1K-blocks"}`), `-P` prints `1024-blocks` (`df.go:185`) and the GNU header
word `Use%` (`df.go:216`) instead of `Capacity`, and `-t` is bound to GNU
`--type` filesystem filtering requiring an argument (`df.go:70`), so
`df -t` alone is a loud rc=2 flag error and never produces the required
allocated-space figures. GNU `--total` is a separate grand-total extension,
not an implementation of POSIX `-t`. Environment: `LANG`, `LC_ALL`,
`LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`; locale-sensitive argument and
message handling is unimplemented — implementation_gap. Evidence:
`cmds/df/df_test.go#TestDefaultListing`,
`#TestKFlagSameUnits`, `#TestPortablePrintTypeAndInodesHeaders`,
`#TestTypeFiltersAndTotal`, `#TestNonexistentOperand`, `#TestUsePct`,
`#TestUnknownFlag`. Status: **partial**, with three implementation gaps as
ranked (units default, `-P` 512/Capacity wording, `-t` repurposed) plus
locale; the `-t` repurposing is the most severe finding in this batch because an
upstream spelling carries a different documented meaning.

## diff

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/diff.html>

Synopsis `diff [-c|-e|-f|-u] [-br] file1 file2` (options XSI) plus `-C n`/
`-U n`; `-` operand is stdin (exactly one); directories recurse with
`Only in`/`Common subdirectories`; exit 0/1/>1. Normal, context, unified,
and ed formats, `-b` blank-run equality, and recursion are verified.
Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `LC_TIME`, `TZ`,
and XSI `NLSPATH`. `TZ` is
honored in `-c`/`-u` headers and tested
(`cmds/diff/diff_test.go#TestHeadersHonorEnvTZMinuteOffsets`); LC_TIME
header rendering is fixed C-locale — implementation_gap; locale-sensitive
character processing, translated messages, and catalog lookup are absent —
implementation_gap. Evidence: `cmds/diff/diff.go`;
`cmds/diff/diff_test.go#TestNormalFormat`, `#TestUnifiedGolden`,
`#TestContextGolden`, `#TestEdFormats`, `#TestRecursiveGolden`,
`#TestBrief`, `#TestIgnoreFlags`, `#TestStdin`, `#TestErrors`. The
generated ledger's warning that attached `-C=<n>`/`-U=<n>` forms are
missing was re-checked and is a false positive (both spellings parse;
tests cover `-C`/`-U` with arguments). Status: **partial**; the cited formats,
options, statuses, and `TZ` behavior are verified, while locale gaps remain.

## dirname

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html>

Synopsis `dirname string`: prefix up to (not including) the last `/`, else
`.`; all-slash strings yield `/`; exit 0/>0. Verified:
`cmds/dirname/dirname.go`; `cmds/dirname/dirname_test.go#TestDirname`,
`#TestDirnameErrors`. Multiple operands and `-z` are GNU extensions under
upstream spellings — noted, out of POSIX scope. Environment: `LANG`,
`LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`; locale-sensitive
argument interpretation, translated diagnostics, and catalog lookup are
unimplemented. Status: **partial**; pathname/result/exit behavior is verified,
while the locale surface remains an implementation gap.

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
(`cmds/du/du.go:434` `"%s\t%s%s"`). Environment: `LANG`, `LC_ALL`,
`LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`; locale-sensitive pathname and
message handling is unimplemented. Status: **partial**; options and symlink
modes are verified, while units, separator, and locale gaps remain.

## env

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html>

Synopsis `env [-i] [name=value]... [utility [argument...]]`; `-` as first
operand is unspecified and treated as `-i` (GNU-compatible, acceptable);
PATH from the modified environment locates `utility` (verified); exit
status passes through from `utility`, else 0/1-125/126 not
executable/127 not found — these execution/status elements are verified. env
is one of the repo's documented spawn-exceptions (the operand IS the utility).
Evidence:
`cmds/env/env.go`; `cmds/env/env_test.go#TestEnv`,
`#TestEnvRunsCommandWithModifiedEnvironment`,
`#TestEnvCommandStdioAndExitStatus`,
`#TestEnvCommandNotFoundAndNotExecutable`, `#TestEnvErrors`;
`cmds/env/env_exec_test.go#TestEnvExecPassesArgvVerbatimWithoutShell`,
`#TestEnvExecEmptyPathSearchesWorkingDirectoryOnly`. Environment
(affecting env's own diagnostics): `LANG`, `LC_ALL`, `LC_CTYPE`,
`LC_MESSAGES`, and XSI `NLSPATH`; translated diagnostics/catalog lookup are
unimplemented. Status: **partial**; environment mutation, utility execution,
and statuses are verified, while the message-locale surface remains a gap.

## expand

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html>

Synopsis `expand [-t tablist] [file...]`; tablist is a single positive
decimal or comma/blank-separated strictly ascending list; a tab at or
beyond the last stop expands to a single space; `-` operand is stdin;
exit 0/>0 with a diagnostic on operand access failure. These elements are
verified:
`cmds/expand/expand.go`; `cmds/expand/expand_test.go#TestExpandDefaultTabsFromStdin`,
`#TestExpandCustomTabsAndFile`, `#TestExpandTabListIncrement`,
`#TestExpandBlankSeparatedTabList`,
`#TestExpandTabsBeyondLastStopBecomeSingleSpaces`, `#TestExpandRejectsBadTabs`.
Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`.
Gap: locale-directed blank/column classification is unimplemented (fixed
UTF-8/byte logic —
`#TestExpandWideRuneCountsDisplayColumns`,
`#TestExpandCombiningMarkIsZeroWidth` cover the C/UTF-8 paths only) —
implementation_gap; translated messages/catalog lookup are absent —
implementation_gap. Status: **partial**; tab-list/file/stdio behavior is
verified, while locale gaps remain.

## expr

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html>

Operators `| & = > >= < <= != + - * / % :` with precedence and parens;
anchored BRE matching for `:`; decimal output with leading zeroes
removed; exit 0 result non-null/non-zero, 1 null/zero, 2 invalid
expression, >2 error (division by zero → 3). These non-locale elements are
verified:
`cmds/expr/expr.go`; `cmds/expr/expr_test.go#TestExprArithmetic`,
`#TestExprMatch`, `#TestExprPOSIXArithmeticAndComparison`,
`#TestExprPOSIXBooleanAndExitStatus`, `#TestExprPOSIXMatchAndStringFunctions`.
Environment: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and
XSI `NLSPATH`. Gaps: locale-directed string comparison and BRE
collation/character classes are unimplemented (byte/C behavior), and
translated messages/catalog lookup are absent — implementation_gap. GNU
keyword tokens (`index length match substr`) are extensions. Status:
**partial**; operators, precedence, results, and statuses are verified, while
locale gaps remain.

## file

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/file.html>

Synopsis `file [-dh] [-M file] [-m file] file...` and `file -i [-h]
file...`; `-d` default position-sensitive tests, `-h` no dereference,
`-i` minimal identification, `-M file`/`-m file` magic files; `-`
as stdin is an implementation-defined choice, which this implementation
supports. Output is `<pathname>: <type>` with required
strings `cannot open`, and descriptions containing `commands text`
(shell scripts) and `c program text` (C sources); exit 0 on successful
completion, >0 on error. The standard explicitly requires a `cannot open`
classification for nonexistent/unreadable operands but does not state that
this classified outcome alone makes completion unsuccessful; the current
exit 0 is therefore not labeled a conformance gap without an authoritative
interpretation.

Implementation: `-h` is bound and tested. `-m` is bound, but its parser
accepts only numeric-offset `s`/`string` literal tests and therefore covers
only a narrow subset of the required magic-file grammar
(`cmds/file/file_test.go#TestAdditionalMagicFileFallsBackToBuiltins`) —
implementation_gap. `-d`, `-i`, and `-M` are unbound and fail loudly —
implementation_gap; their required order-sensitive interactions with `-m`
are consequently absent. The default is no-dereference
(`cmds/file/file.go:27` flags help text
"(default)", `file.go:65` `Lstat`) where POSIX resolves symlinks by
default — implementation_gap. Type strings use GNU wording
(`script, text executable`, `C source`) not the required containment —
implementation_gap. Environment: `LANG`, `LC_ALL`, `LC_CTYPE`,
`LC_MESSAGES`, and XSI `NLSPATH`; locale-directed content classification,
translated messages, and catalog lookup are absent — implementation_gap.
Evidence: `cmds/file/file.go`;
`cmds/file/file_test.go#TestTextEmptyDataAndStdin`,
`#TestPortableSignatures`, `#TestDirectorySymlinkAndFollow`,
`#TestNoDereferenceShortOptionAndTrailingSlash`,
`#TestErrorsAndStrictFlags`. Status: **partial**; `-h`, the stdin choice, and
the cited built-in classifications are verified, while options/magic grammar,
default symlink handling, type strings, and locale gaps remain.

## find

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html>

Synopsis `find [-H|-L] path... [operand_expression...]`; by default symbolic
links are not followed. The accepted leading `-P` spelling names that default
but is an extension, not an Issue 7 option. The exact Issue 7 primaries are `-name`, `-path`,
`-nouser`, `-nogroup`, `-xdev`, `-prune`, `-depth`, `-perm`, `-type`,
`-links`, `-user`, `-group`, `-size`, `-atime`, `-ctime`, `-mtime`, `-exec`,
`-ok`, `-print`, and `-newer`, with operators `!`, `-a`, `-o`, `(`, `)`.
These are implemented. `-print0` is an implemented GNU extension;
`-lname`, `-inum`, `-mount`, `-local`, and `-follow` are non-POSIX names and
are not implemented here, so none is POSIX evidence. The `{} +` form makes a
failing invocation affect find's exit status; `-ok` evaluates the effective
`LC_MESSAGES` `yesexpr`, rather than accepting a fixed `y`; exit is 0 when all
paths are traversed, >0 on traversal/error conditions. Evidence:
`cmds/find/find.go`; `cmds/find/find_test.go#TestFindTests`,
`#TestFindDefaultPath`, `#TestFindPrint0`, `#TestFindMtimeAndNewer`,
`#TestFindTypeSymlink`, `#TestFindDepthOrder`, `#TestFindXdev`,
`#TestFindPerm`, `#TestFindUserGroup`, `#TestFindErrors`;
`cmds/find/exec_test.go#TestFindExecSemicolon`, `#TestFindExecPlus`,
`#TestFindExecStatusSemantics`, `#TestFindOk`;
`cmds/find/conformance_test.go#TestFindPatternsAreCLocaleAndLocaleInvariant`.
Note (not a gap):
`-exec ... \;` child failure does not affect exit status — exactly what
POSIX specifies (only `{} +` mandates nonzero); GNU additionally
returns 1, so this is a GNU deviation, not a POSIX one.

Environment: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and
XSI `NLSPATH`. C/POSIX matching is byte-oriented and tested. This worktree
also implements the certification locale de_DE ISO-8859-1: equivalence
classes honor `LC_COLLATE`, character classes honor `LC_CTYPE`, and `-ok`
honors the `LC_MESSAGES` affirmative expression, with category precedence
verified by `cmds/find/find_test.go#TestFindGermanLocaleCategories`,
`#TestFindGermanAffirmativeResponse`, and `#TestFindVSCLocalePrecedence`.
Translated diagnostics and catalog lookup remain implementation gaps. Status:
**partial**; the exact POSIX options/primaries/operators, execution semantics,
and supported locale effects are verified, while message-catalog behavior
remains a gap.
