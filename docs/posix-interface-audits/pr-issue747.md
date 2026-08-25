# `pr` POSIX.1-2008 Issue 7 interface audit

Normative source: [The Open Group Issue 7, 2016 Edition `pr`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pr.html).
GNU-only options remain extensions and are not used as evidence for this audit.

## Required interface and executable evidence

| Requirement | Implementation evidence | Test evidence |
| --- | --- | --- |
| `+page`, protected `-- +file`, page suppression | `protectPlusOperands`, `parsePages`, `inPageRange` | `TestPRPlusOperandPageRange`, `TestPRDoubleDashProtectsPlusOperand`, `TestPRPagesRangeAndDateFormat` |
| `-column`, vertical fill and balancing | `scanColumnOption`, `printColumns`, `printColumnChunk` | `TestPRVerticalColumns`, `TestPRVerticalColumnsUnevenFinalPage`, `TestPRClusteredColumnWithFlags` |
| `-a` across fill | `options.across`, `printColumnChunk` | `TestPRAcrossColumns`, `TestPRVerticalColumnsInteractions` |
| `-d` double spacing | `options.doubleSpace` body budgeting and emitters | `TestPRNumberIndentAndDoubleSpace`, `TestPRVerticalColumnsInteractions` |
| `-e[char][gap]` attached optional argument, zero/default gap, expansion | `scanColumnOption`, `parseOptionalCharNumber`, `expandChars` | `TestPRExpandTabs`, `TestPROptionalExpandArgument`, `TestPROptionalGapZeroMeansDefault` |
| `-f` XSI form feed plus first-page terminal pause; `-F` form feed | `pager`, `pause`, `formFeed` | `TestPRFormFeedLowerEqualsUpper`, `TestPRFormFeedLowerPausesOnlyBeforeFirstPageOnTerminal`, `TestPRFormFeedTrailer` |
| `-h header` | `headerSet`, `headerLine` | `TestPRCustomHeaderAndTOmitPagination` |
| `-i[char][gap]` attached optional argument and output tab replacement | `scanColumnOption`, `parseOptionalCharNumber`, `tabifyLine` | `TestPRClusteredOptionalOutputTabs`, `TestPROptionalGapZeroMeansDefault`, `TestPRMultiColumnImpliesExpandAndOutputTabs` |
| `-l lines`, including header/trailer suppression when `lines <= 10` | page/body budgeting | `TestPRDefaultPageStructure`, `TestPRShortPageLengthImpliesOmitHeader`, `TestPRRejectsBadLength` |
| `-m` parallel merge of at least nine operands | `runMerge`, `printMerge`, `mergeLine` | `TestPRMerge`, `TestPRMergeThreeFilesAndPagination`, `TestPRMergeSeparatorOddWidthDoesNotReservePadding` |
| `-n[char][width]` | `parseOptionalCharNumber`, `formatLine` | `TestPROptionalNumberArgument`, `TestPRMergeLineNumbering` |
| `-o offset` | `formatLine`, header indentation | `TestPRNumberIndentAndDoubleSpace`, `TestPRRejectsBadIndent` |
| `-p` terminal alert and `/dev/tty` carriage-return wait before every page | `pager.before`, `pause`, platform control-terminal providers | `TestPRPausePerPageWritesAlertAndReadsDevTTY`, `TestPRPauseAcceptsCarriageReturnFromDevTTY`, `TestPRPauseFlagsNoOpWhenStdoutIsNotATerminal` |
| `-r` suppresses open diagnostics without hiding failure status | `noFileWarnings` | `TestPRNoFileWarnings` |
| `-s[char]`, including bare-tab default and 512-column default width | `NoOptDefVal`, `separator` width selection | `TestPRBareSeparatorDoesNotConsumeFile`, `TestPRCustomSeparatorTruncatesColumns`, `TestPRSeparatorDefaultWidthIs512` |
| `-t` headers/trailers omitted and no last-page padding | `omitHeader` | `TestPROmitHeaderPassesContent`, `TestPRInputFormFeedsBreakPages` |
| `-w width` affects multiple-column output only; single-column input is not truncated | column width calculations | `TestPRSingleColumnNeverTruncatedByDefault`, `TestPRColumnsReserveBlankBetweenFullWidthCells`, `TestPRRejectsMoreColumnsThanPageWidth` |
| Multiple-column output assumes `-e` and `-i`, including `-m` | option normalization before all emitters | `TestPRMultiColumnImpliesExpandAndOutputTabs`, `TestPRMergeAssumesPOSIXTabExpansionAndReplacement` |
| File operands in order; absent/`-` operand selects stdin; file mtime versus current time | `open` and serial loop | `TestPRDefaultPageStructure`, `TestPRStreamsCompletePageBeforeEOF`, `TestPRMergeOpenErrorContinuesAndNewline` |
| Exact 66-line default: five-line header, 56 body lines, five-line trailer | page budgeting and emitters | `TestPRDefaultPageStructure`, `TestPRStreamsCompletePageBeforeEOF` |
| POSIX header `"%s %s Page %d"` and POSIX-locale date `%b %e %H:%M %Y` | `headerLine` | `TestPRDefaultPageStructure` |
| `LC_TIME` precedence and `TZ` conversion for headers | `locale.ResolveTime`, `tzenv.Location`, `headerLine` | `TestPRHeaderHonorsInvocationTZAndLCTime` |
| Terminal diagnostics deferred until processing completes | invocation-owned diagnostic buffer | `TestPRTerminalDefersFileDiagnosticsUntilOutputCompletes` (serial and merge) |
| SIGINT during a terminal pause flushes diagnostics and terminates nonzero | `errPauseInterrupted`, `interruptExit` | `TestPRPauseInterrupted` |
| Read, write, short-write, `/dev/tty`, and aggregate-open failures are diagnosed and nonzero | all reader/emitter/flush paths | `TestPRStreamReadAndShortWriteFailuresAreNonzero`, `TestPRMergeFlushErrorStopsBeforeAnotherPause`, `TestPRPauseUnavailableControllingTerminal`, `TestPRPauseDevTTYReadError`, `TestPRMergeOpenErrorContinuesAndNewline` |

## Reconciled findings

The old `go-batch-4.md` concern that `pr` emitted a centered GNU/ISO header is
stale. Current source emits the exact POSIX-locale shape and the golden test
pins `Jan  2 03:04 2020 in Page 1`. This issue also closed three source gaps:
`-m` now receives the required implicit tab expansion/replacement, diagnostics
are deferred when stdout is a terminal, and SIGINT no longer reports success.
The shared bounded `LC_TIME` provider supplies C/POSIX and `de_DE` data and
fails closed for unavailable locales; `TZ` is invocation-local.

## Honest residuals

The command remains `partial`, not globally verified. Display-column
calculation and truncation are byte-oriented, so full `LC_CTYPE` multibyte
character width and printable-class behavior remain open. The embedded
`LC_TIME` corpus is bounded rather than a complete installed-locale database.
Translated `LC_MESSAGES` diagnostics and `NLSPATH` catalogs are absent.
Terminal pause coverage is hermetic through the real `/dev/tty` provider seam;
an end-to-end controlling-terminal run is still required on the Ubuntu
certification host. These residuals do not conceal a missing required option
or operand form.

Focused acceptance gates: `go test -count=20 ./cmds/pr`,
`go test -race -count=5 ./cmds/pr`, `go vet ./cmds/pr`, plus repository
cross-build/vet after integration.
