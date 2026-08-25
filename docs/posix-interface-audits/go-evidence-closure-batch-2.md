# POSIX Go evidence closure: owned batch 2

This audit covers `cmp`, `date`, `iconv`, `id`, `logger`, `logname`, `nohup`,
`paste`, `touch`, and `uname` against POSIX.1-2016 (Issue 7). The normative
sources are the Open Group command pages linked below. GNU behavior is not
used as POSIX evidence.

The ledger rows now carry the exact applicable synopsis, options, operands,
stream behavior, side effects, status contract, and command-specific test
references. All ten rows are deliberately `partial`, not `verified`: every
page carries `LC_MESSAGES` and the XSI `NLSPATH` catalog behavior, and none of
these packages implements translated diagnostics or message catalogs. The
command-specific residuals below make the remaining boundary explicit.

## Verdicts and exact residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `cmp` | [Issue 7 cmp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cmp.html) | partial | Translated diagnostics absent. This batch fixes the `-l` STDOUT format: in POSIX mode (`POSIXLY_CORRECT` present) every difference is exactly `"%d %o %o"`; outside POSIX mode the GNU-diffutils aligned columns are retained as the recorded upstream-extension rendering, per `docs/reference-policy.md` ("POSIX behavior wins" applies in POSIX mode; cmp's extension family is GNU Diffutils, not Coreutils). The previously untested `-l` unequal-length `cmp: EOF on %s` stderr requirement now has a focused test. Remaining: the GNU operand extensions (one-operand default to `-`, trailing SKIP1/SKIP2) mean one-, three-, and four-operand invocations that strict Issue 7 would reject are accepted; mid-stream read-error branches and `-s` with an unreadable file remain untested; the `-n 0` EOF diagnostic mislabels a non-empty file as empty (GNU-extension path only). |
| `date` | [Issue 7 date](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html) | partial | The XSI `date [-u] mmddhhmm[[cc]yy]` form now validates every field and real calendar date, applies the current-year and 69/68 two-digit-year rules, interprets wall time through invocation `TZ` (with `-u` overriding it), invokes an injectable platform clock setter exactly once, propagates setter/output failures, and writes the resulting default-format date. Linux, Darwin, AIX, DragonFly, FreeBSD, NetBSD, and OpenBSD use `settimeofday`; other platforms fail loudly. `TZ` unset/null uses the system default as the date page requires. `LC_TIME` precedence is invocation-local and the bounded C/POSIX and de_DE UTF-8/ISO-8859-1 providers cover names plus `%c`, `%x`, `%X`, `%r`, `%p`, `%h`, and default output; unavailable locales fail before clock mutation. Evidence: `TestDateXSISetDateOperand`, `TestDateXSISetDateRejectsBeforeMutationAndPropagatesFailure`, `TestDateLCTimeCompleteFormatsAndUnsupported`, `TestDateLCTimePrecedence`, and `TestDateTZ`. Residuals: actual clock mutation depends on privilege and kernel support; Solaris and non-Unix targets refuse it. The locale corpus is intentionally bounded, translated diagnostics and `NLSPATH` catalogs are absent, `%S` cannot represent leap second 60 in Go, and non-POSIX `-d` can produce years outside the `%C`/`%y` Issue 7 range. |
| `iconv` | [Issue 7 iconv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html) | partial | Translated diagnostics absent. The charmap-file synopsis form (`-f frommap -t tomap`, a `/` in the value) is not implemented; it is rejected loudly before any I/O. Locale-default codesets for omitted `-f`/`-t` come from a small carried table, not `nl_langinfo(CODESET)`. This batch fixes two `-c` discard-tracking losses (an empty later operand no longer launders an earlier operand's failure status; a GB18030 four-byte sequence truncated at EOF now counts as a discard). Remaining and known: the "-c shall not affect the exit status" clause is still violated for lenient decoders — `iconv -f UTF-8 -t UTF-8` over ill-formed input exits 0 while the same input with `-c` exits 1, because the non-`-c` lane performs U+FFFD replacement instead of detecting ill-formed input; closing it requires a strict-validation layer across decoders, deliberately out of this batch's bounds. `-s` also over-suppresses read-side I/O diagnostics. `-l` under-reports the accepted set, and the `CP858` alias maps to the unregistered name `IBM858` and is refused (extension surface). |
| `id` | [Issue 7 id](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html) | partial | Translated diagnostics absent. `-r` with `-G` is accepted as a deliberate BSD/GNU-documented extension beyond the Issue 7 synopsis (where `-r` pairs only with `-g`/`-u`); the combination is output-neutral because `-G` already lists every distinct ID. A numeric operand resolves via the user database as an extension. The named-operand default format, `id USER` group listing, and forced lookup-failure name fallbacks are implemented but untested; real/effective divergence is only provable through the `processIDsFn` seam because the test process is never setuid; the `!unix` build cannot satisfy the `%u` formats and has no coverage. |
| `logger` | [Issue 7 logger](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html) | partial | Translated diagnostics absent. POSIX leaves the destination implementation-defined: on unix the message goes to the local syslog socket (a host with no syslog listener fails loudly with status 1), and on Windows every invocation fails loudly — the Windows path has no build-tagged test. The zero-operand stdin form and the `-i`/`-s`/`-t`/`-p` options are documented non-POSIX extensions; a leading-dash operand is parsed as an option unless `--` precedes it, a strict-syntax deviation shared with the framework's interspersed parsing. "The message is actually saved" is evidenced only up to the injectable sink boundary. |
| `logname` | [Issue 7 logname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html) | partial | Translated diagnostics absent. The name comes only from a `getlogin()`-equivalent (Linux audit loginuid mapped through the user database) and never from the environment — `LOGNAME`/`USER` decoys are pinned ignored. Residual: on every non-Linux platform the command currently always fails with `logname: no login name` (exit 1), because no pure-Go `getlogin()` equivalent is wired there; the in-repo `cmds/internal/session` utmpx reader is a plausible future darwin source, so this is a closable gap, not a platform impossibility. On Linux, sessions without an audit loginuid (containers, non-PAM) fail where `getlogin()` would succeed. The success-path `"%s\n"` output is asserted only when the host actually has a login name; both success tests skip otherwise. |
| `nohup` | [Issue 7 nohup](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html) | partial | Translated diagnostics absent. The 126/127 exit partition, SIGHUP immunity of both the utility and the nohup invocation itself, terminal stdout/stderr redirection to `nohup.out` (invocation dir then `$HOME`), the required appending notice, and environment passthrough are all pinned by tests; the earlier 125/`POSIXLY_CORRECT` design was removed in 9c85d14 and status 127 is now unconditional, confirmed by test across environments. Residuals: `--help`/`--version` as a sole argument answer about nohup instead of invoking a utility of that name (GNU-shaped extension); `nohup.out` create mode passes 0600 to open but is not re-chmodded against the umask, and no test stats the created mode or proves append-after-existing-content; the "neither file can be opened ⇒ utility not invoked" double-failure case and the non-ENOENT start-failure branch (currently 126) are untested; a terminal stdin is redirected to a write-only /dev/null, so the child reads EBADF rather than EOF (within the "may redirect from an unspecified file" latitude, pinned by test); pty-dependent tests skip silently where no pty exists. |
| `paste` | [Issue 7 paste](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html) | partial | Translated diagnostics absent. Parallel and serial modes, delimiter cycling and per-line/per-file reset, `\n`/`\t`/`\\`/`\0` escapes, EOF-as-empty-lines, the `-s` empty-file bare newline, circular multi-`-` stdin reads (parallel), and the no-output-on-open-failure rule are pinned. Residuals: zero operands default to stdin (extension beyond the `file...` synopsis); delimiter lists are decoded as UTF-8 unconditionally, and a test pins that non-locale-aware behavior although LC_CTYPE is normative; repeated `-` under `-s` sits on a genuine OPERANDS/`-s` clause conflict, resolved silently (first `-` drains stdin) and untested; the 12-operand minimum, `\\` escape, serial `\0`, mid-file read errors, and stdout write failure are untested. |
| `touch` | [Issue 7 touch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/touch.html) | partial | Translated diagnostics absent. This batch fixes the `-t` SS=60 clause: 60 now means one second after :59 at every position, including 23:59 where the stamp rolls into the next day/month/year (previously "invalid date format"). Remaining: `-t` stamps before the portable signed-32-bit boundary are rejected as a syntax error (an implementation limit reported with a misleading diagnostic, deliberately pinned by test); the `--stamp` long spelling bypasses the one-time-source exclusivity that `-t` enforces; `-r` atime propagation is untested everywhere and wrong on non-linux/darwin/windows builds (`atime_other.go` substitutes mtime); creation mode 0666, `-c` on existing files, multi-operand error continuation, and `-t` field-range rejections are untested. |
| `uname` | [Issue 7 uname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html) | partial | Translated diagnostics absent. Fixed field order, `-a` ≡ `-mnrsv` with extensions excluded, and operand rejection are pinned. This batch closes the Windows `-v` omission: the implementation-defined version symbol is now `Build N` from `RtlGetVersion`, while output assembly defensively avoids empty fields and repeated selectors. The Windows-tagged test pins the non-empty version contract and is compiled by the cross-OS gate; probe values are otherwise asserted for internal consistency rather than against a separate reference uname, utsname whitespace runs are collapsed, and no Windows runtime runner is available in this gate. |

## Fixes applied in this batch

Bounded, confirmed-defect fixes only; no GNU 9.11 broadening:

- `cmds/date` — the XSI set-date operand is parsed and executed through a
  hermetic clock-setter boundary; null/unset `TZ`, `-u`, all optional-year
  forms, validation-before-mutation, setter failure, and successful localized
  output have focused evidence. The de_DE provider now carries its complete
  `%c`/`%x`/`%X`/`%r`/`%p` behavior and unsupported `LC_TIME` fails closed.

- `cmds/cmp/cmp.go` — `-l` emits the exact Issue 7 `"%d %o %o"` format when
  `POSIXLY_CORRECT` is present in the invocation environment; the
  GNU-diffutils aligned columns remain the non-POSIX default.
  Tests: `cmp_posix_test.go#TestCmpVerbosePOSIXModeFormat`,
  `#TestCmpVerboseEOFDiagnostic` (the latter also closes the previously
  untested `-l` EOF stderr requirement).
- `cmds/touch/touch.go` — `parseStamp` builds SS=60 stamps at :59 and adds
  one second after validation, so day/month/year boundaries roll forward per
  the Issue 7 `-t` clause. Test:
  `touch_stamp_boundary_test.go#TestTouchStampSecond60RollsForward`.
- `cmds/iconv/iconv.go` — a discard recorded by an earlier operand is no
  longer cleared by a later empty UTF-16 operand, and a GB18030 four-byte
  sequence truncated at EOF now records its discard. Tests:
  `iconv_discard_state_test.go#TestDiscardStatusSurvivesLaterEmptyOperand`,
  `#TestDiscardTruncatedGB18030FourByteTailFails`.
- `cmds/uname/uname.go` and `uname_windows.go` — output assembly extracted
  into `assemble()` and Windows now supplies the required implementation-
  defined `-v` symbol as `Build N` from `RtlGetVersion`; repeated selectors
  do not duplicate fields. Tests:
  `uname_assemble_test.go#TestAssembleSkipsSyntheticEmptyVersion`,
  `#TestAssembleFixedOrderWithVersion`, and
  `uname_windows_test.go#TestWindowsProbeHasPOSIXVersion`.

All four fixes were verified fail-closed: the new tests fail against the
pre-fix sources and pass after.

## Independent confirmation of the recent correction commits

- 56dc05f/8cf7d8c (`date`/`iconv`/`id`/`logger`/`logname`): confirmed in
  current source and by live probes. `date`'s XSI operand is refused loudly
  with an empty stdout; `logname` ignores `LOGNAME`/`USER` and fails with
  `logname: no login name` where no audit loginuid exists; `id -r` alone and
  `-n` alone stay usage errors while `-rG` is the documented extension.
- 0740f27 (`iconv` failure status with discarded characters): confirmed —
  `-c`/`-cs` keep exit 1 on discards; this batch closes two paths where that
  status was still lost, and records the remaining lenient-decoder asymmetry
  as an explicit residual rather than closure.
- 9c85d14 + 76cc77d + 731e7cc (`cmp`/`nohup`/`paste`/`touch`/`uname`):
  confirmed by test and probe — `nohup` internal failures are 127
  unconditionally (125/`POSIXLY_CORRECT` removed), the invocation itself
  survives SIGHUP, `paste -s` emits a bare newline for an empty file,
  `touch` treats `-` as an ordinary pathname, and `uname -a` excludes `-o`
  while `-a -v` equals `-a`; Windows now also supplies a non-empty `-v`
  implementation string.

## Supersessions of stale audit statements

- `go-batch-6.md` and `sprint-79-consolidated.md` still describe `uname -a`
  as appending the GNU `-o` field; commit 9c85d14 fixed that and
  `uname_test.go#TestUnameAllIsExactlyMNRSV` pins it. Those documents are
  point-in-time snapshots and are superseded here.
- `go-batch-3.md` cites `iconv_test.go#TestUnsupportedEncodingAndDiscardOptionFailLoudly`
  and `#TestMissingEncodingIsUsageError`, which no longer exist, and records
  "`-c` refused", which is no longer true; the ledger row now cites the
  current test set.
