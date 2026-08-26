# Sprint 79: POSIX required-command interface status

This report reconciles the Sprint 79 interface ledger through coreutils `43d7a78`
(including the Profile C source waves and issues 781-784) against POSIX.1-2016
Issue 7, current source, command-package tests, and the
sibling `sh` and `bashy` evidence repositories. The canonical machine-readable
source is [`posix-required-command-interfaces.tsv`](../posix-required-command-interfaces.tsv).
GNU extension behavior is not certification evidence.

## Exact denominator and current verdict

The ledger contains exactly 116 required names: 78 effective Go-owned, 22
effective shell-owned, and 16 external-provider-owned. The current evidence
states are **0 verified, 3 implemented, 97 partial, and 16 missing**.

| Effective owner | Verified | Implemented | Partial | Missing | Total |
| --- | ---: | ---: | ---: | ---: | ---: |
| Go | 0 | 1 | 77 | 0 | 78 |
| Shell | 0 | 2 | 20 | 0 | 22 |
| External provider | 0 | 0 | 0 | 16 | 16 |
| Total | 0 | 3 | 97 | 16 | 116 |

The implemented owned interfaces are shell-selected `false` and `true`, plus
Go-selected `nice`. They are not verified because the byte-derived proprietary
integration verification gate was not rerun for this reconciliation. The two status-only
shell interfaces have both
command-specific semantic evidence in `sh` and command-specific Profile B
routing evidence in `bashy`. `nice` is promoted on the accepted process-level
barrier evidence at canonical `afee303`: user code cannot begin before the
priority attempt, adjustment refusal still invokes the utility unchanged, and
utility/126/127/signal statuses, argv, environment, and streams are exact. No
Go row is promoted merely because its package suite is dense.

## Exact owned inventory

The 78 Go-owned names, covered once by the accepted six inventory batches,
are:

| Batch | Commands |
| --- | --- |
| 1 | `at awk basename batch cat chgrp chmod chown cksum cmp comm cp crontab` |
| 2 | `csplit cut date dd df diff dirname du env expand expr file find` |
| 3 | `fold getconf grep head iconv id join ln locale logger logname ls mesg` |
| 4 | `mkdir mkfifo more mv newgrp nice nohup od paste pathchk pax pr ps` |
| 5 | `renice rm rmdir sed sleep sort split strings stty tabs tail tee touch` |
| 6 | `tput tr tsort tty uname unexpand uniq uudecode uuencode wc who write xargs` |

The 22 shell-owned names are `alias bg cd command echo false fc fg getopts
hash jobs kill printf pwd read sh test time true umask unalias wait`. The 16
provider rows are outside `--require-owned-complete` and remain separately
missing.

## Fail-closed evidence decision

Every Go row now contains a complete candidate transcription for synopsis,
options and option arguments, operands and special tokens, environment,
standard streams, effects/output files, status, and all required XCU clause
IDs. Every cited Go reference names an existing `Test...` declaration in that
command's package. This is an inventory result, not a conformance result.

The earlier universal per-command message-catalog blocker was too broad and is
withdrawn. Issue 7 has four distinct requirements:

1. Each utility's `ENVIRONMENT VARIABLES` clause says the listed variables
   affect execution. `LC_ALL` precedence over `LC_MESSAGES` is mandatory, and
   a recognized selected locale must be honored. The XBD
   [Internationalization Variables](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/basedefs/V1_chap08.html)
   clause also says an unrecognized locale has unspecified behavior.
2. `LC_MESSAGES` selects affirmative/negative response rules and the language
   and cultural conventions in which messages *should* be written. The XCU
   [`STDERR` default](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap01.html)
   likewise uses *should*, not *shall*, for unspecified-format diagnostic and
   informative messages.
3. `NLSPATH` determines catalog lookup through `catopen()` only where XSI is
   applicable. The ledger now records it explicitly as `xsi:NLSPATH`; it is
   not an unconditional Base-profile variable.
4. Neither clause requires every utility to own or ship a translated catalog,
   much less two synthetic catalogs. Absence of shipped translations is a
   localization product gap, not by itself a failed POSIX utility interface.

Certification remains fail-closed on observable requirements. Every command
must still prove its required diagnostics, stream, continuation, and status
behavior in the POSIX locale. Commands whose normative behavior depends on
`LC_MESSAGES` data (for example affirmative-response parsing) need focused
locale tests. A shared locale/catalog subsystem may prove category precedence
and XSI lookup once, with command-specific evidence that the utility actually
uses that subsystem; repeated two-catalog fixtures in all 78 packages are not
required. This correction alone promotes no row: every partial owned row still
has one or more command-specific residuals below or in its closure audit.

## Ranked command-specific residuals

The table records the highest-yield command-specific blocker.
The accepted per-command closure audits remain authoritative for additional
lower-ranked edges.

| Rank | Command(s) | Missing clause | Concrete product behavior and focused test needed |
| ---: | --- | --- | --- |
| 1 | `bg`, `fg` | `EFFECTS`, `EXIT_STATUS`, terminal/job-control consequences | Accepted sibling-sh tests now prove parser/default selection, real process-group continuation, foreground terminal ownership, reads, waits, restoration, and completed status. Still add stopped/signaled/multi-job failure products, disabled-job-control behavior through the selected process route, required diagnostics/statuses, and platform-specific job-control disposition. |
| 1 | `jobs` | `STDOUT`, `EXIT_STATUS` | `TestJobsIssue7ParserAndFormatting` proves required parsing and principal formats. Still add asynchronous real job-table transitions for every required state/signal rendering, multiple-process `-l`, invalid/stale job IDs, locale strings, and output failures. |
| 1 | `fc` | `OPERANDS`, `STDOUT`, `EFFECTS`, `EXIT_STATUS` | Accepted tests prove form validation, first-only substitution, and multiline listing. Still add real editor invocation, forward/reverse/clamped ranges, history persistence limits, command re-execution state and status, `FCEDIT`/`HISTFILE`/`HISTSIZE`, and I/O/locale failures. |
| 1 | `sh` | shell-language clauses beyond entrypoint selection | The process-level Bashy test proves the selected `sh` utility route, `-c`, script-file and stdin forms, `$0`/positional arguments, empty command, missing-file status, and argv0 strict-POSIX engagement. It does not prove the complete shell grammar, expansions, redirections, traps, environments, interactive/job-control behavior, and every 1-127 status consequence; retain partial. |
| 2 | `at`, `batch`, `crontab` | host scheduler integration | Accepted Issue 743 source now proves strict allow/deny policy parsing and fail-closed stat errors, empty-deny access, unknown-job listing status, correct `-t` diagnostics, and crontab backslash/percent translation. Existing tests cover persisted environment/cwd/umask, queue/load markers, atomic installation, daemon handoff, mail-provider routing, and carried `LC_TIME`/`TZ` forms. Remaining: real system policy directories and privilege products, live mail delivery/load gating, and installed-locale breadth. |
| 2 | `awk`, `comm`, `csplit`, `expr`, `fold`, `grep`, `join`, `sed`, `sort`, `unexpand`, `uniq`, `wc` | locale-provider and integration breadth | Locale waves A-C now provide canonical command-surface evidence for the applicable `LC_CTYPE`, `LC_COLLATE`, and `LC_NUMERIC` behavior in the bounded C/POSIX, supported UTF-8, and carried `de_DE.ISO-8859-1` products. Unsupported provider/platform combinations fail closed. These rows remain partial for arbitrary installed-locale/provider breadth and the unrepeated proprietary integration gate; the implemented bounded-locale semantics must not be described as absent. |
| 2 | `tr` | locale-provider and integration breadth | Issues 769 and 781 implement the carried multibyte character model; binary-value `-c`; LC_CTYPE-character `-C` ordered by LC_COLLATE; ranges, equivalence, and `[c*]`; post-transform character squeezing; raw-byte preservation; bounded writes; and provider preflight/lifecycle. The repaired `-C` domain includes every carried LC_CTYPE character, including NUL and bytes that LC_COLLATE cannot name as collating elements. Remaining: arbitrary installed locales, a general multibyte collation provider, and integration evidence. |
| 2 | `cut` | locale-provider integration | Accepted source now applies invocation `LC_CTYPE` to `-c`, `-b -n`, and multibyte `-d` boundaries while preserving exact input bytes; focused tests cover C/POSIX, UTF-8, ISO-8859-1, malformed input, precedence, long lines, and fail-before-I/O behavior. Remaining: the carried locale corpus is bounded and installed locales outside it fail closed. |
| 2 | `expand` | locale-provider integration | Accepted source now retains exact byte spans and uses invocation `LC_CTYPE` for display-column accounting; focused tests cover C/POSIX, UTF-8 widths, ISO-8859-1, malformed input, precedence, `-i`, read errors, and short writes. Remaining: the carried locale corpus and Unicode width policy are bounded. |
| 2 | `date` | XSI `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Issue 748 and manager review now prove every Issue 7 conversion and E/O fallback in the carried locales, `LC_TIME` precedence/fail-closed behavior, invocation `TZ`/`-u`, validation before the injected clock setter, XSI year rules, setter failure, and output error plus short-write status. Remaining: privileged real clock-set integration, leap-second rendering from a system clock source, additional platform setters, and installed locales outside the bounded C/POSIX and `de_DE` corpus. |
| 2 | `getconf` | platform integration | Accepted source now inventories every mandatory sysconf/pathconf/confstr/minimum name, routes pathname and system queries, distinguishes undefined results, validates programming environments, and propagates path/output errors. Remaining: privileged/kernel-limit products, a non-Linux/Darwin runtime provider, and broader native platform fixtures. |
| 2 | `file` | locale/platform integration | Accepted Issue 746 evidence maps every required option (`-d`, `-h`, `-i`, `-M`, `-m`), option ordering, operands/stdin choice, symlink policy, default and custom magic grammar, required type strings, inaccessible operands, status, and output failures. Manager review rejected an attempted `-b` behavior change because `-b` is not an Issue 7 option. Remaining: broader locale providers, platform-specific device wording, and implementation-defined type-string breadth. |
| 2 | `find` | platform/locale integration | Accepted source now requires a path operand whenever `POSIXLY_CORRECT` is present while preserving the default-`.` extension outside POSIX mode. Focused products cover every required primary/action/operator, real and seam-backed ownership cases, locale precedence for patterns and `-ok`, `-exec` side effects/batching, traversal failures, and aggregate status. Remaining: cross-mount positive `-xdev`, non-Unix identity providers, and locale/filesystem breadth outside the carried corpus. |
| 2 | `id` | credential/platform integration | Accepted Issue 745 tests now prove named-user default and every selector/name combination, exact name-or-number fallback, portable accounts without supplementary groups, and stdout error/short-write status. Remaining: a real set-ID process fixture and a conformant numeric-credential provider or explicit failure disposition on every non-Unix target. |
| 2 | `logname` | session/platform integration | Accepted Issue 745 source retains Linux audit-login-UID resolution, adds a native cgo-free `getlogin` provider for Darwin/DragonFly/FreeBSD, proves a real Darwin login session, and keeps required no-login, environment/effective-user isolation, and output-error products. Remaining: live Linux target-session coverage, runtime DragonFly/FreeBSD coverage, and providers for the explicitly unsupported POSIX targets. |
| 2 | `od` | locale-provider integration | Accepted source now applies invocation-owned `LC_CTYPE` to `-t c`, including printable UTF-8 first-byte/`**` continuation fields across output groups, exact Latin-1 bytes, malformed/nonprintable octals, and bounded streaming lookahead. `LC_NUMERIC` controls the radix across all required floating type strings and carried ABIs. Remaining: the locale corpus and Unicode printability tables are bounded and translated catalogs are absent. |
| 2 | `paste` | locale-provider integration | Accepted source now splits `-d LIST` into delimiter characters per invocation `LC_CTYPE` (carried C/POSIX, their UTF-8 aliases, and `de_DE.ISO-8859-1`, original bytes preserved, unsupported locales failing before any operand opens), and has focused tests for repeated `-` under `-s`, the twelve-operand minimum, the `\\` escape, serial `\0`, mid-file read errors, and stdout write/short-write failures. Remaining: locale codeset discovery is a bounded carried corpus rather than `nl_langinfo(CODESET)`; unqualified installed locales outside that corpus fail closed. |
| 2 | `pathchk` | filesystem/platform integration | Accepted source now handles failed and indeterminate containing-filesystem limit queries, differing limits at depth, diagnostics, and aggregate status. Linux runtime evidence proves filesystem-valid non-UTF-8 component bytes are not incorrectly rejected by a UTF-8 locale. Remaining: a side-effect-free provider for missing-name syntax on filesystems with additional encoding restrictions and non-Linux/Darwin runtime coverage. |
| 2 | `pr` | `ENVIRONMENT_VARIABLES`, platform/locale integration | Issue 747 closes the complete required option/optional-argument matrix, corrects the ledger's XSI classification (`-f`, not `-l`), proves exact headers and page structures, makes `-m` assume `-e`/`-i`, applies invocation `TZ` and bounded `LC_TIME`, defers terminal diagnostics, returns nonzero on SIGINT, and covers read/write/short-write failures. Remaining: multibyte `LC_CTYPE` display widths/printability, installed locales outside the carried corpus, and a real controlling-terminal target-host run. |
| 2 | `tty` | platform/locale integration | Linux and Darwin have real PTY terminal-name tests, with Darwin `/dev/console` coverage; the exact POSIX-locale nonterminal output, status partition, invalid descriptors, output errors, and short writes are covered. Remaining: truthful terminal pathname lookup on the other POSIX targets, an integration-host controlling-terminal run, and locale message providers outside the POSIX locale. |
| 2 | `more` | `ENVIRONMENT_VARIABLES`, terminal integration | The accepted implementation now covers the full Issue 7 option and interactive command grammar, `$MORE`/`LINES`/`COLUMNS`/`EDITOR`/`TERM`, tag/search/editor behavior, terminal overstrikes, and I/O failures. Remaining boundaries are unavailable UTF-8 collation providers and terminal-capability/platform integration—not absent translated catalogs or unsupported `-i`, `-p`, or `-t`. |
| 2 | `cp` | `OUTPUT_FILES`, platform/error integration | Accepted source now rejects unsafe/aliased destinations, preserves physical symlink metadata without mutating referents, preserves portable atime or fails loudly, and has exact same-file/destination/umask/PATH_MAX tests. Issue 779 keeps POSIX `-i` and `-f` independently effective in either order; GNU last-option-wins remains extension-only outside POSIX mode. Remaining: privileged device-node and ownership products, injected mid-copy read/write/unlink failures, non-Linux/Darwin symlink metadata, and Windows runtime paths. |
| 2 | `ls` | locale/terminal/platform integration | Issue 777 proves last-`-H`/`-L` ordering across clusters and spellings, and sticky output-error/short-write propagation across listing formats, help/version, continuation, and recursion. Remaining: non-C `LC_COLLATE`/`LC_TIME`, terminal width/capability discovery, and non-Unix runtime metadata behavior. |
| 2 | `touch` | `OUTPUT_FILES`, platform/error integration | Accepted source now obtains reference atime on supported stat layouts and fails loudly when unavailable; literal `-`, TZ, leap second, near-PATH_MAX, and reference-time paths are tested. Remaining: real atime propagation on every target, 0666 creation through umask, `-c` existing-file and multi-operand failure products, and full range rejection. |
| 2 | `iconv` | `ENVIRONMENT_VARIABLES`, locale-provider integration | Accepted source implements pathname charmaps, strict malformed/truncated detection across every carried multibyte family, `-c` status invariance, exact `-s` scope, locale-derived omitted encodings, aliases, file/stdin ordering, and read/write failures. Remaining: locale codeset discovery is a deterministic carried corpus rather than `nl_langinfo(CODESET)`; unknown unqualified installed locales fail closed. |
| 3 | `chgrp`, `chown`, `chmod`, `mkdir`, `mkfifo`, `mv`, `rm`, `uudecode`, `uuencode` | `OUTPUT_FILES`, `CONSEQUENCES_OF_ERRORS` | Add privilege-contained kernel/filesystem tests for ownership, mode/set-ID/ctime, symlink/hard-link identity, special files, permission failures, and platform-specific implementations rather than seam-only proof. |
| 3 | `df`, `du` | `STDOUT`, `CONSEQUENCES_OF_ERRORS` | Add mounted cross-device fixtures, real mount discovery, hard-link products, free-slot/space accounting, output failures, and platform-specific formats. |
| 3 | `logger` | `EFFECTS`, `EXIT_STATUS` | Add a real local syslog receiver and Windows disposition test; prove message persistence, zero-operand stdin behavior, open/send/close errors, and statuses. |
| 3 | `nohup` | `OUTPUT_FILES`, `EXIT_STATUS` | Issue 779 closes the no-option operand boundary: in POSIX mode sole `--`, `--help`, and `--version` are utility operands; outside POSIX mode standalone help/version remain extensions, with `nohup -- --help` the explicit dash-name form. Remaining: `nohup.out` mode/append evidence, both output-open failures preventing execution, non-ENOENT start failures, and non-skipping PTY paths. |
| 3 | `ps` | `STDOUT`, `ENVIRONMENT_VARIABLES` | Add runtime providers for non-Linux targets and tests for every required field, identity lookup, unavailable-data representation, terminal width, locale time, and live selection semantics. |
| 3 | `newgrp` | `EFFECTS`, privilege/host integration | Accepted source proves name-before-numeric lookup, membership/password/TTY policy, supplementary capacity fallback, equal real/effective GID plans, unchanged-shell retry after refusal, login argv0/environment/cwd, virtual umask, streams, and shell status/signal propagation. Remaining: a real setuid-root credential transition, attended controlling-terminal password entry, NSS/PAM policy, and non-Unix disposition are host/privilege integration boundaries. |
| 3 | `pax` | `OUTPUT_FILES`, platform/filesystem integration | Accepted issues 715-717, 775, 776, and 778 cover interactive/substitution preflight, copy links, source-atime reset, ordered preservation, the full `-o` families, archive-sink block-size classification, verbose hard/symbolic-link `ls -l` fields and occurrence-bound targets, effective custom-list PAX names, write errors, and deterministic six-month/future timestamp selection. Remaining: special-file extraction, privileged ownership/device products, unsupported no-follow metadata platforms, real terminal breadth, and uncarried legacy locale encodings. |
| 3 | `renice` | `EFFECTS`, `EXIT_STATUS` | Add a hermetic scheduler seam for exact `which` dispatch and mixed ID success, plus privilege-contained real priority-change tests and Windows disposition. |
| 3 | `stty`, `tabs`, `tput` | terminal `STDIN`/`STDOUT`, `EFFECTS` | Run required forms against real terminal/terminfo databases across supported platforms, including Windows disposition, unavailable capabilities, atomic state, write errors, and exact statuses. |
| 3 | `who`, `write` | `INPUT_FILES`, `OUTPUT_FILES`, terminal effects | Add live login-database and PTY fixtures across supported ABIs, credentials, terminal ownership/activity/permission state, interruption, framing, close errors, and platform fail-closed behavior. |
| 4 | `cat` | command-specific stream/status edge | Accepted tests now prove injected mid-stream stdin read failures through both the copy and line loops (diagnostic naming the operand, status 1, later operands still processed), directory and dangling-symlink operands with continuation, `/dev/null` alone and interleaved, and FIFO-through-symlink streaming. Remaining: unlocalized diagnostics in non-C locales (superseded product gap), Windows special-file behavior, and the process-level SIGPIPE/default broken-pipe disposition, which is not recorded as evidence (XCU cat ASYNCHRONOUS EVENTS is Default). |
| 4 | `tee` | platform signal/filesystem integration | Issue 764 proves retained extensions do not displace required options; invocation-umask file creation; unbuffered streaming; input read errors; stdout and per-file open/write/short-write/close errors with continuation; and Unix process-level default SIGINT, default SIGPIPE, and `-i` behavior. Remaining: Windows signal behavior and real close-error products on additional filesystems/platforms. |
| 4 | `tsort` | platform/input integration | Issue 764 corrects the token grammar to exact space/tab/newline separators, pins graph and cycle diagnostics/continuation/status, and proves input read/close plus output error/short-write failures while retaining non-conflicting extensions. Remaining: cross-platform runtime filesystem/error products and the documented bounded-token implementation limit. |
| 4 | `uname` | platform-provider integration | Issue 764 proves exact selector composition/order/deduplication, retained non-conflicting extensions, provider and unavailable-field failure, and output error/short-write status. Remaining: runtime provider evidence across Linux, Windows, BSD, AIX, and Solaris rather than cross-build evidence alone. |
| 4 | `basename`, `cksum`, `cmp`, `dirname`, `env`, `head`, `ln`, `mesg`, `rmdir`, `sleep`, `split`, `strings`, `tail`, `xargs` | command-specific stream/status edge | The closure audits find the main C/POSIX algorithm supportable, but each row still names concrete unproved I/O, signal, special-file, platform, or locale branches. Add exactly the missing observable test listed in its closure audit; no synthetic per-command catalog fixture is imposed. |

Detailed residual wording is in the accepted `go-evidence-closure-batch-1.md`
through `go-evidence-closure-batch-7c.md` reports. Their command-specific
behavioral gaps remain useful, but any statement that absence of translated
diagnostics or per-command `NLSPATH` catalogs alone blocks verification is
superseded by the normative policy above.

## Shell-selected routing and evidence

Profile C selects GNU Bash invoked as `sh`; Profile D selects Bashy's
argv0=`sh` strict POSIX route. The same-name Go applets are therefore not the
direct shell owner for `echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`time`, and `true`.

All 22 shell names have currently resolvable, command-specific semantic and
routing references. `false` and `true` are implemented, not verified because
the byte-derived integration verification gate was not rerun. The other twenty
remain partial because their closure audits identify concrete locale, interactive,
process, filesystem, or grammar residuals. The new `bg`, `fc`, `fg`, and `jobs`
references are
command-specific sibling-sh tests from `c354d6fc`; the `sh` row additionally
uses Bashy's process-level entrypoint contract alongside its two independent
route/strict-mode tests. The runner accepts only an explicit `sh` semantic root
and explicit Bashy routing root, rejects wrong-root and cross-lane evidence,
and records each resolved revision and dirty-state hash in its contract. The
final setup used `sh` at `6b123d57` and Bashy at `29513d6`; retained workspace
copies are not authoritative evidence roots. These path-and-test references do
not prove complete clause coverage: that is why twenty rows remain partial.

## Accepted source-wave reconciliation

This report is reconciled through canonical `43d7a78`, including the Profile C
source waves, accepted Issue 781 repair, Linux evidence reconciliation, and
final hermetic `id` evidence correction. It credits the accepted command
waves and their current test declarations: `more` through `b899308`, `4e78606`,
`27031a7`, and `bf800b4`; `touch` through `0b4950b` and `b1da07b`; `cp` through
`014684e`, `f523c76`, and `266f353`; `nice` through `afee303` and `4b6beb8`;
`newgrp` through `2c459cc` and `0d24315`; `pax` issues 715-717 through
`b614e20`, `27c4c30`, `8b7c051`, `6cd55bc`, `af1fd00`, `ddbe753`,
`7fff4b6`, `4741bf9`, `dd63476`, `3aefbac`, `9ef3a43`, and matrix refresh
`dc79ecd`; `iconv` through `70bc09b` and matrix refresh `056e48a`; `fold`
through `e461654` and matrix refresh `c354351`; `getconf` through
`ceae89d`, `2fbc91c`, and matrix refresh `07e9f17`; `expand` through
`e038148` and matrix refresh `befd0dc`; `cut` through `185851f` and matrix
refresh `110c1f4`; and `paste` through `7a93e44` and matrix refresh `94a4a7b`;
and `cat` read-error/special-file evidence through `a843aea` and review
amendment `4c3d133`; `od` locale rendering through `231f687`; `pathchk`
filesystem-limit handling through `eb4c01c`; and `find` POSIX operand routing
and clause evidence through `1a27ec0`; scheduler access/list/cron translation
through `e5cda57`; `file` mandatory magic/option evidence through `92fbfb3`
with the rejected non-POSIX `-b` change recorded by `e3b37e2`; and `id`/`logname`
output, portable-account, and BSD session-provider evidence through `49e8fab`,
`232c333`, and manager amendment `9561a21`.
It additionally credits `date` Issue 748 through `8d83339` and manager
correction `a2e79d4` (removing limitation-locking tests and adding short-write
handling), and `pr` Issue 747 through manager-completed `987058e`, merged by
`4f8684c` and `1ae7adc`. The `tty` Issue 749 worker made no source change;
manager reruns independently passed count-20, race-5, vet, and Linux, Darwin,
Windows, AIX, and FreeBSD cross-build coverage against the existing tests.

The later Profile C reconciliation credits locale wave A through `1886238`,
`5548b53`, and `e2c109f`; locale wave B through `96b25a0`, `ae8ff9b`,
`5cda0c2`, and `525acc1`; locale wave C Issue 769 through `fdab2de`; and awk
numeric-radix plus expr back-reference closure through Issue 780 commit
`c418161`. The awk dependency is the public `github.com/qiangli/goawk` fork at
commit `88712e61a085`, pinned as
`v0.0.0-20260826042810-88712e61a085`; its added public surface is
`interp.Config.DecimalPoint`. Program literals and `-v` assignments retain the
portable period spelling, while input conversion and numeric output use the
selected carried radix. Issue 779's cp/nohup option-boundary fixes are
`bcd6c42`; Issue 777's final ls changes are `5761c57` and `a5f14fc`; the pax
verbose-list and timestamp closures are `1de6d83`, `598f8d3`, and `e3cbd8f`,
with archive-sink block sizing at `ed4ed06`. Issue 781's final accepted repair
is `9e53b12`; its audit reconciliation is `4326c9e`. Issue 782 makes the
Ubuntu/Linux Go lane target-runnable, and issue 784 removes a host-account-name
assumption from `id` evidence without changing production behavior.

The machine-readable evidence lanes retain their validated test IDs, including
the nice pre-exec barrier/failure path, newgrp credential/login/umask helper,
pax interactive/preservation/extended-header families, iconv
charmap/stream/status matrix, fold locale/byte/error matrix, getconf
inventory/platform/error matrix, cut and expand locale/byte/error matrices,
paste locale-delimiter/error matrix, and cat injected-read/special-file
continuation evidence. The canonical TSV includes the accepted issue 780
`awk`/`expr` and issue 781 `tr` TestIDs. On the pinned Ubuntu 24.04 image at
final coreutils `43d7a78`, a non-root, globally POSIX, network-disabled run
passed all 78 Go command events and all 1,086 exact TestIDs, with zero missing,
skipped, or failed references. This is public source-interface evidence, not a
proprietary certification rerun and not grounds for promoting a row to
`verified`.
These recently accepted locale/platform waves remain partial because their
locale-provider and platform residuals are stated explicitly above.

## Reproduction

```sh
python3 scripts/posix_manifest.py --check
python3 scripts/posix_manifest.py --require-owned-source-complete
python3 scripts/posix_manifest.py --require-owned-complete
python3 -m unittest scripts/posix_manifest_test.py
scripts/applet-matrix.py --check
python3 scripts/posix_interface_runner.py --state-dir /state/profile-c --owner go --json
```

The owned-source-completion command is expected to fail at this snapshot with
97 partial rows. The final owned-completion command fails all 100 owned rows:
three are implemented but cannot be promoted before the deferred integration gate exists,
and 97 remain partial.
Those failures are the truthful Sprint 79 residual, not waived gates.
