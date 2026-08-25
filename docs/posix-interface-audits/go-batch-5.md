# POSIX Interface Audit: Go Batch 5

This fail-closed audit covers exactly 13 Go-owned commands against POSIX.1-2016 (Issue 7): `renice`, `rm`, `rmdir`, `sed`, `sleep`, `sort`, `split`, `strings`, `stty`, `tabs`, `tail`, `tee`, and `touch`. Its canonical implementation baseline is coreutils `1523713`. A package test passing proves only the behavior it asserts; untested mandatory behavior is an `evidence_gap`, and source-confirmed divergence is an `implementation_gap`.

## Ranked confirmed gaps

1. **`sed` — critical:** non-C BRE collation/equivalence/collating-element and back-reference routing is incomplete; corrective issue 52 remains pending review.
2. **`sort` — high:** `-bdfi` character handling is fixed ASCII/byte logic rather than `LC_CTYPE` behavior, and supported locale coverage is narrow.
3. **`touch` — high:** the mandatory pathname operand `-` is rejected as unsupported.
4. **`rm` — high:** canonical commits `2102a00` and `51eeaee` close implicit write-protection prompting, including prompting before protected-directory descent, but affirmative responses still ignore `LC_MESSAGES` `yesexpr`.
5. **`strings` — medium:** canonical commits `a84afe9` and `4879065` make scanning character-granular for C/POSIX, UTF-8, and the reviewed single-byte provider, but the provider deliberately rejects other non-UTF-8 locales.
6. **Evidence gaps:** `renice`, `rmdir`, `sleep`, `split`, `stty`, `tabs`, `tail`, and `tee` have supportable core implementations but retain focused portability, terminal-provider, signal, error, or locale evidence gaps. In particular, canonical `stty` commit `1523713` closes the mandatory core interface and atomicity defects; its residuals are evidence/locale coverage, not the former interface gap.

Canonical fixes reflected here are `2102a00` and `51eeaee` (with descendants) for `renice`/`rm`, `a84afe9` and `4879065` for `strings`/`tabs`, and accepted/integrated `1523713` for `stty`. Pending issue-52 commit `f601cae` for `sed` is recorded for traceability but is not counted as canonical evidence.

## `renice`

**POSIX specification:** [Issue 7/2016: renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html)

- **Interface:** `renice [-g|-p|-u] -n increment ID...`. `-g` selects process groups, `-p` processes (default), and `-u` users; `increment` is a signed decimal integer, while numeric IDs are unsigned decimal integers. Stdin/stdout are unused; diagnostics go to stderr; exit is 0 on success and greater than 0 on error.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`; the confirmed numeric-ID implementation defect is closed canonically.
- **Source evidence:** [`cmds/renice/renice.go`](../../cmds/renice/renice.go) uses `strconv.ParseUint` for numeric IDs, selects the correct target class, applies an increment, clamps to implementation limits, continues operands, and reports failures. This is the canonical result of `2102a00` (descended from the issue-36 work).
- **Test evidence:** [`cmds/renice/renice_test.go`](../../cmds/renice/renice_test.go), notably `TestInvalidIDsAreRejected`, `TestMutuallyExclusiveSelectors`, `TestMissingIncrementAndMissingID`, and `TestZeroIncrementPreservesCurrentValue`, closes signed/non-decimal ID parsing and the stable process path. Privilege- and host-dependent `-g`/`-u` execution plus locale-sensitive diagnostics remain outside focused hermetic evidence.

## `rm`

**POSIX specification:** [Issue 7/2016: rm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rm.html)

- **Interfaces:** `rm [-iRr] file...` and `rm -f [-iRr] [file...]`. `-f` suppresses missing-file/no-operand failures and earlier `-i`; `-i` prompts and overrides earlier `-f`; `-R` and `-r` recurse. Stdin supplies prompt responses; prompts/diagnostics use stderr; no normal stdout; exit is 0 only when all required removals succeed.
- **Environment:** `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`; `LC_MESSAGES` supplies the affirmative-response expression.
- **Status:** `implementation_gap`.
- **Source evidence:** [`cmds/rm/rm.go`](../../cmds/rm/rm.go) correctly preserves `rm -f` with zero operands, option precedence, dot/dot-dot/root refusal, non-following recursive traversal, continuation, and the terminal/unwritable implicit prompt before file removal or recursive protected-directory descent. Canonical commits `2102a00` and `51eeaee` close those prompt-order defects. `confirm` still accepts only hardcoded `y`, `Y`, or `yes`, ignoring locale `yesexpr`.
- **Test evidence:** [`cmds/rm/rm_test.go`](../../cmds/rm/rm_test.go) includes `TestRmImplicitPromptForUnwritable`, `TestRmImplicitDirectoryPromptPrecedesDescent`, `TestRmInteractivePrompt`, `TestRmRecursiveInteractivePrompts`, `TestRmLastPromptOptionWins`, and `TestRmInteractiveAfterForceNeedsOperand`. These close the canonical prompt behavior but do not close the locale affirmative-response gap.

## `rmdir`

**POSIX specification:** [Issue 7/2016: rmdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html)

- **Interface:** `rmdir [-p] dir...`; `-p` removes each named directory and then eligible ancestors. Stdin/stdout are unused, diagnostics go to stderr, and exit is 0 on success and greater than 0 on error.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`.
- **Source evidence:** [`cmds/rmdir/rmdir.go`](../../cmds/rmdir/rmdir.go) requires operands, removes only directories, implements ancestor removal, rejects final `.`/`..`, stops each parent walk on failure, continues later operands, and accumulates nonzero status.
- **Test evidence:** [`cmds/rmdir/rmdir_test.go`](../../cmds/rmdir/rmdir_test.go) has stable coverage in `TestRmdirParents`, `TestRmdirParentsStopsOnNonEmpty`, `TestRmdirContinuesPastErrors`, `TestRmdirTrailingDotComponent`, and `TestRmdirTrailingDotDotComponent`. No focused evidence establishes locale-sensitive diagnostics or every `-p` permission/error edge, so a fail-closed audit does not promote it to verified.

## `sed`

**POSIX specification:** [Issue 7/2016: sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html)

- **Interfaces:** `sed [-n] script [file...]`; `sed [-n] -e script [-e script]... [-f script_file]... [file...]`; and `sed [-n] [-e script]... -f script_file [-f script_file]... [file...]`. `-n` suppresses default output; `-e` and `-f` add scripts in command order. No file, and each file operand `-`, selects stdin. Output is the edited stream; diagnostics use stderr; exit is 0 on success and greater than 0 on error.
- **Environment:** `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `implementation_gap` (corrective issue 52 pending review/integration).
- **Source evidence:** [`cmds/sed/sed.go`](../../cmds/sed/sed.go), [`cmds/sed/internal/gosed`](../../cmds/sed/internal/gosed), and [`pkg/bre`](../../pkg/bre) implement the command language, ordered `-e`/`-f`, stdin `-`, C/POSIX BRE, and a narrow non-C `LC_CTYPE` route. The canonical source does not independently route `LC_COLLATE`, and non-C ranges, equivalence/collating constructs, and valid BRE back-references remain incomplete.
- **Test evidence:** [`cmds/sed/sed_test.go`](../../cmds/sed/sed_test.go) includes `TestSedPreservesMixedExpressionFileOrder`, `TestSedBREBackrefConformance`, and broad command coverage. [`cmds/sed/ctype_test.go`](../../cmds/sed/ctype_test.go) proves only the current provider slice and fail-closed errors, not full Issue-7 locale semantics.
- **Pending fix:** issue-52 commit `f601cae` addresses separate category routing/ranges/backrefs but is still under review; multi-character collating elements remain deliberately fail-closed.

## `sleep`

**POSIX specification:** [Issue 7/2016: sleep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html)

- **Interface:** `sleep time`, with exactly one non-negative decimal-integer seconds operand and no options. It uses no streams on success, suspends for at least the requested interval, and returns 0 on completion or greater than 0 on error. SIGALRM may complete successfully, be ignored, or take its default action; other signals take their standard actions.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`.
- **Source evidence:** [`cmds/sleep/sleep.go`](../../cmds/sleep/sleep.go) correctly accepts the mandatory integer subset and waits with a timer. Fractional/suffixed/multiple operands are extensions, not POSIX requirements, and do not remove the required one-integer behavior.
- **Test evidence:** [`cmds/sleep/sleep_test.go`](../../cmds/sleep/sleep_test.go) covers parsing, short waits, errors, and embedding-context cancellation in `TestSleepZeroish`, `TestSleepSuffixMath`, and `TestSleepErrors`. It does not provide a focused integral-duration lower-bound or real SIGALRM behavior test.

## `sort`

**POSIX specification:** [Issue 7/2016: sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html)

- **Interfaces:** `sort [-m] [-o output] [-bdfinru] [-t char] [-k keydef]... [file...]` and `sort [-c|-C] [-bdfinru] [-t char] [-k keydef] [file]`. Each `-` file is stdin. Sorting/merging writes stdout unless `-o`; checking writes no stdout; status distinguishes ordered, disorder, and error.
- **Environment:** `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `LC_NUMERIC`, `NLSPATH`.
- **Status:** `implementation_gap`.
- **Source evidence:** [`cmds/sort/sort.go`](../../cmds/sort/sort.go), [`cmds/sort/collator.go`](../../cmds/sort/collator.go), and [`cmds/sort/compare.go`](../../cmds/sort/compare.go) implement both forms, repeated stdin, merge/check/output, collation-provider routing, and narrow numeric locale support. However, `normalizeTextKey` hardcodes ASCII blanks, alphanumeric, printable, and case folding for mandatory `-b`, `-d`, `-i`, and `-f`, rather than using `LC_CTYPE`; public non-C locale support is also narrower than the standard interface.
- **Test evidence:** [`cmds/sort/sort_test.go`](../../cmds/sort/sort_test.go) includes `TestOpenOperandsPreservesSharedStdinAndClosesFiles`, `TestSortKeys`, `TestSortCheck`, `TestSortMerge`, and `TestSortLCNumeric`. [`cmds/sort/collator_test.go`](../../cmds/sort/collator_test.go) proves injected collation behavior but not full `LC_CTYPE` semantics.

## `split`

**POSIX specification:** [Issue 7/2016: split](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html)

- **Interfaces:** `split [-l line_count] [-a suffix_length] [file [name]]` and `split -b n[k|m] [-a suffix_length] [file [name]]`. The default and explicit file `-` select stdin; output pieces use `name` (default `x`) plus generated suffixes; stdout is unused; diagnostics use stderr; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`.
- **Source evidence:** [`cmds/split/split.go`](../../cmds/split/split.go) implements both standard modes, `-a`, defaults, exact operand arity, stdin `-`, suffix generation/exhaustion, output creation, and failure propagation. GNU-only modes are extensions and are not counted as POSIX features.
- **Test evidence:** [`cmds/split/split_test.go`](../../cmds/split/split_test.go) includes `TestSplitLines`, `TestSplitBytes`, `TestSplitNumericAndSuffixLen`, `TestSplitSuffixExhaustion`, `TestSplitOperandsAndPrefix`, and `TestSplitErrors`. It does not inject input read or output close failures, so all mandated error consequences are not stably evidenced.

## `strings`

**POSIX specification:** [Issue 7/2016: strings](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html)

- **Interface:** `strings [-a] [-t format] [-n number] [file...]`; `format` is `d`, `o`, or `x`; `number` counts printable characters. With no file it reads stdin. A first argument `-` has unspecified results and must not be advertised as mandated stdin behavior. Output contains qualifying strings and optional byte offsets; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `implementation_gap` only for the deliberately limited non-UTF-8 locale-provider set; the former byte-counting defect is closed canonically.
- **Source evidence:** [`cmds/strings/strings.go`](../../cmds/strings/strings.go) implements options, files, offsets, continuation, write errors, character-granular `-n`, exact byte preservation/offsets in UTF-8, C/POSIX classification, and reviewed single-byte `LC_CTYPE` classification. It correctly treats operand `-` as a pathname because POSIX leaves a first `-` unspecified rather than requiring stdin. Canonical commits `a84afe9` and `4879065` supply these fixes. [`pkg/ctype/ctype.go`](../../pkg/ctype/ctype.go) deliberately supports only C/POSIX and two ISO-8859-1 aliases and rejects other non-UTF-8 locale names.
- **Test evidence:** [`cmds/strings/strings_test.go`](../../cmds/strings/strings_test.go) includes `TestStringsDashPathname`, `TestStringsUTF8`, `TestStringsUTF8ReplacementCharacterPreservesBytesAndOffset`, `TestStringsFiles`, and `TestStringsErrors`. It proves character counts, malformed UTF-8 boundaries, the valid replacement character, and byte offsets; it does not turn the limited provider set into general non-UTF-8 locale support.

## `stty`

**POSIX specification:** [Issue 7/2016: stty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html)

- **Interfaces:** `stty [-a|-g]` for reports, or `stty operand...` for settings. Mandatory operands cover speeds; control/input/output/local modes and their negations; delay modes; `min`/`time`; rows/columns; and `eof`, `eol`, `erase`, `intr`, `kill`, `quit`, `susp`, `start`, and `stop` control characters. Stdin identifies the terminal; reports use stdout; diagnostics use stderr; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`; independently accepted issue-53 work is integrated canonically as `1523713`, closing the mandatory core-interface defect.
- **Source evidence:** [`cmds/stty/stty.go`](../../cmds/stty/stty.go), [`cmds/stty/stty_termios.go`](../../cmds/stty/stty_termios.go), [`cmds/stty/stty_linux.go`](../../cmds/stty/stty_linux.go), and [`cmds/stty/stty_bsd.go`](../../cmds/stty/stty_bsd.go) implement mandatory speeds, modes and negations, delays, `min`/`time`, rows/columns, control characters, complete kernel-derived `-a`, versioned restorable `-g`, prevalidation, and rollback on application failure. POSIX terminal platforms have explicit termios backends; unsupported platforms fail closed.
- **Test evidence:** [`cmds/stty/stty_test.go`](../../cmds/stty/stty_test.go), [`cmds/stty/stty_termios_test.go`](../../cmds/stty/stty_termios_test.go), and [`cmds/stty/stty_posix_test.go`](../../cmds/stty/stty_posix_test.go) exercise required modes against a PTY, `-a`, `-g` restore, speeds/control characters, decimal parsing, platform `_POSIX_VDISABLE`, invalid-later-operand atomicity, overflow, window sizes, and Linux PTY speed storage. Residual evidence is platform execution outside the exercised Linux/Darwin PTY paths and locale-sensitive diagnostic/display behavior; it is not a missing core operand/interface claim.

## `tabs`

**POSIX specification:** [Issue 7/2016: tabs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tabs.html)

- **Interfaces:** `tabs [-n|-a|-a2|-c|-c2|-c3|-f|-p|-s|-u] [-T type]` and `tabs [-T type] n[[sep[+]n]...]`; presets are XSI-shaded, `-n` is the single-digit repetitive form, and operands are ascending decimal stops with optional relative `+n` after the first. Stdin is unused; terminal-control output goes to stdout; diagnostics use stderr; exit is 0 or greater than 0.
- **Environment:** `TERM` plus `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`. With neither `-T` nor a usable `TERM`, an unspecified supported default terminal type is required.
- **Status:** `evidence_gap`; the confirmed unset/null `TERM` defect is closed canonically.
- **Source evidence:** [`cmds/tabs/tabs.go`](../../cmds/tabs/tabs.go) implements repetitive/preset/explicit/incremental stops, defaults an absent `-T`/`TERM` to the supported `ansi` entry, and preserves its published `+m[n]` margin and leading `+[n]` compatibility forms; canonical commit `a84afe9` supplies the terminal fallback without deleting those established forms.
- **Test evidence:** [`cmds/tabs/tabs_test.go`](../../cmds/tabs/tabs_test.go) includes `TestPresetColumnsMatchPOSIX`, `TestRepetitiveSpec`, `TestIncrementForm`, `TestMargin`, and `TestTerminalTypeResolution`, including absent-`TERM` fallback. Remaining evidence is limited to the reviewed terminfo capability fixtures/system availability and does not establish every supported terminal type or locale-sensitive diagnostic.

## `tail`

**POSIX specification:** [Issue 7/2016: tail](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html)

- **Interface:** `tail [-f] [-c number|-n number] [file]`; signed decimal `number` selects from the start (`+`) or end, and default output is the last ten lines. No file and file `-` select stdin. `-f` follows named regular files/FIFOs, but is ignored for pipe/FIFO stdin. Output goes to stdout; diagnostics use stderr; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`; the Issue-7 functional path is strongly supported, but locale-sensitive diagnostics are not stably evidenced.
- **Source evidence:** [`cmds/tail/tail.go`](../../cmds/tail/tail.go) implements signed line/byte counts, the default, stdin `-`, read/write errors, descriptor following, named FIFO behavior, and the required ignore rule for piped stdin. Multi-file/header and other flags are GNU extensions, not claimed as POSIX requirements.
- **Test evidence:** [`cmds/tail/tail_test.go`](../../cmds/tail/tail_test.go) includes `TestTail`, `TestTailFollowDescriptor`, `TestTailFollowStdinPipeIgnoresFollow`, and `TestTailFollowStdinRegularFile`; [`cmds/tail/tail_fifo_unix_test.go`](../../cmds/tail/tail_fifo_unix_test.go) provides `TestTailFollowFIFOAcrossWriters`.

## `tee`

**POSIX specification:** [Issue 7/2016: tee](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html)

- **Interface:** `tee [-ai] [file...]`; `-a` appends and `-i` ignores SIGINT. Stdin is copied to stdout and every named file; `-` is a literal filename, not stdout. File failures are diagnosed while other outputs continue; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
- **Status:** `evidence_gap`; the Issue-7 functional path is strongly supported, but locale-sensitive diagnostics are not stably evidenced.
- **Source evidence:** [`cmds/tee/tee.go`](../../cmds/tee/tee.go) implements append/truncate, literal `-`, SIGINT ignoring, per-sink continuation, input/output/close errors, and nonzero status. GNU output-error modes are extensions and are excluded from the POSIX disposition.
- **Test evidence:** [`cmds/tee/tee_test.go`](../../cmds/tee/tee_test.go) includes `TestTeeWritesFiles`, `TestTeeAppend`, `TestTeeOpenErrorContinues`, `TestTeeDashIsLiteralFileName`, and `TestTeeStdoutWriteErrorPOSIX`; [`cmds/tee/run_signal_test.go`](../../cmds/tee/run_signal_test.go) supplies the real-signal `TestTeeIgnoreInterruptsActual`.

## `touch`

**POSIX specification:** [Issue 7/2016: touch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/touch.html)

- **Interface:** `touch [-acm] [-r ref_file|-t time|-d date_time] file...`; `-a`/`-m` select timestamps, `-c` suppresses creation, and `-r`, `-t`, and `-d` are mutually exclusive time sources. Stdin/stdout are unused; diagnostics use stderr; exit is 0 or greater than 0.
- **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`, `TZ`.
- **Status:** `implementation_gap`.
- **Source evidence:** [`cmds/touch/touch.go`](../../cmds/touch/touch.go) implements the required options, current-time permission semantics, creation, partial timestamp changes, reference files, portable `-t`, fixed-format `-d`, `TZ`, continuation, and failures. However, it explicitly rejects operand `-`; POSIX defines `file` as a pathname, so `-` must be handled as the pathname `-`, not reserved or rejected.
- **Test evidence:** [`cmds/touch/touch_test.go`](../../cmds/touch/touch_test.go) includes `TestTouchCreates`, `TestTouchStamp`, `TestTouchStampUsesInvocationTZAndCurrentYear`, `TestTouchReference`, `TestTouchAccessOnly`, `TestTouchCurrentTimePartial`, and `TestTouchStampPOSIXTZ`. No test covers the mandatory literal `-` pathname behavior.
