# Sprint 79: POSIX required-command interface status

This report reconciles the Sprint 79 interface ledger through coreutils `4c3d133`
against POSIX.1-2016 Issue 7, current source, command-package tests, and the
sibling `sh` and `bashy` evidence repositories. The canonical machine-readable
source is [`posix-required-command-interfaces.tsv`](../posix-required-command-interfaces.tsv).
GNU extension behavior is not certification evidence.

## Exact denominator and current verdict

The ledger contains exactly 116 required names: 78 effective Go-owned, 22
effective shell-owned, and 16 external-provider-owned. The current evidence
states are **3 verified, 97 partial, and 16 unverified**.

| Effective owner | Verified | Partial | Unverified | Total |
| --- | ---: | ---: | ---: | ---: |
| Go | 1 | 77 | 0 | 78 |
| Shell | 2 | 20 | 0 | 22 |
| External provider | 0 | 0 | 16 | 16 |
| Total | 3 | 97 | 16 | 116 |

The verified owned interfaces are shell-selected `false` and `true`, plus
Go-selected `nice`. The two status-only shell interfaces have both
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
unverified.

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
| 2 | `at`, `batch`, `crontab` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Add scheduler integration tests for access policy, delivery/mail behavior, load gating, all locale time grammars, daemon handoff, and persisted execution environment. |
| 2 | `awk`, `comm`, `csplit`, `expr`, `fold`, `grep`, `join`, `sed`, `sort`, `tr`, `unexpand`, `uniq`, `wc` | `ENVIRONMENT_VARIABLES`, algorithmic `STDOUT` | Add non-C multibyte `LC_CTYPE`/`LC_COLLATE`/`LC_NUMERIC` fixtures that discriminate character boundaries, classes, equivalence, ranges, ordering, blanks, widths, and numeric rendering for each named command. |
| 2 | `cut` | locale-provider integration | Accepted source now applies invocation `LC_CTYPE` to `-c`, `-b -n`, and multibyte `-d` boundaries while preserving exact input bytes; focused tests cover C/POSIX, UTF-8, ISO-8859-1, malformed input, precedence, long lines, and fail-before-I/O behavior. Remaining: the carried locale corpus is bounded and installed locales outside it fail closed. |
| 2 | `expand` | locale-provider integration | Accepted source now retains exact byte spans and uses invocation `LC_CTYPE` for display-column accounting; focused tests cover C/POSIX, UTF-8 widths, ISO-8859-1, malformed input, precedence, `-i`, read errors, and short writes. Remaining: the carried locale corpus and Unicode width policy are bounded. |
| 2 | `date` | XSI `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Add privileged clock-set integration coverage, leap-second rendering, additional platform setters, and a complete installed-locale `LC_TIME` matrix with mutation-after-validation checks. |
| 2 | `getconf` | platform integration | Accepted source now inventories every mandatory sysconf/pathconf/confstr/minimum name, routes pathname and system queries, distinguishes undefined results, validates programming environments, and propagates path/output errors. Remaining: privileged/kernel-limit products, a non-Linux/Darwin runtime provider, and broader native platform certification fixtures. |
| 2 | `file` | `OPTIONS`, `STDOUT` | Add complete magic-file grammar and required type-string tests, including `-d`, `-i`, `-M`, symlink policy, stdin, inaccessible operands, and locale effects. |
| 2 | `find` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Reject the omitted-path extension in POSIX mode or document a gated route; add all primary/action products, real ownership databases, locale `-ok`/pattern behavior, filesystem failures, and `-exec` side effects. |
| 2 | `id` | `STDOUT` | Add named-user default/group output, lookup-failure fallbacks, a real set-ID process fixture, and executable non-Unix behavior or an explicit platform conformance disposition. |
| 2 | `logname` | `STDOUT`, `EXIT_STATUS` | Current tests prove no effective/environment-user fallback, required no-login failure, RunContext isolation, and output/short-write errors. A real login-session fixture is still required on each supported non-Linux platform; Linux also needs a session where getlogin succeeds without audit loginuid. |
| 2 | `od` | `ENVIRONMENT_VARIABLES`, `STDOUT` | Add a non-C `LC_CTYPE` `-c` rendering fixture and non-C `LC_NUMERIC` floating-format fixture across all required type strings and ABI sizes. |
| 2 | `paste` | locale-provider integration | Accepted source now splits `-d LIST` into delimiter characters per invocation `LC_CTYPE` (carried C/POSIX, their UTF-8 aliases, and `de_DE.ISO-8859-1`, original bytes preserved, unsupported locales failing before any operand opens), and has focused tests for repeated `-` under `-s`, the twelve-operand minimum, the `\\` escape, serial `\0`, mid-file read errors, and stdout write/short-write failures. Remaining: locale codeset discovery is a bounded carried corpus rather than `nl_langinfo(CODESET)`; unqualified installed locales outside that corpus fail closed. |
| 2 | `pathchk` | `OPERANDS`, `EXIT_STATUS` | Current source queries containing-filesystem limits and preserves symlink-before-`..` resolution, with focused tests for both. Still implement and test the required default invalid-byte-sequence check, pathconf failure/indeterminate results, differing mounted limits, and required diagnostics/statuses. |
| 2 | `pr` | `OPTIONS`, `ENVIRONMENT_VARIABLES`, `STDOUT` | Add exact Issue 7 optional-argument grammar, every column/merge/page interaction, locale date/header width, terminal pause/interruption, input/output failure, and status matrix. |
| 2 | `tty` | `STDOUT`, `EXIT_STATUS` | Linux and Darwin now have real terminal-name tests; Windows console behavior, silent mode, invalid descriptors, output errors, and short writes are covered. Add truthful terminal pathname lookup on the remaining POSIX targets and the specified POSIX-locale nonterminal output before verification. |
| 2 | `more` | `ENVIRONMENT_VARIABLES`, terminal integration | The accepted implementation now covers the full Issue 7 option and interactive command grammar, `$MORE`/`LINES`/`COLUMNS`/`EDITOR`/`TERM`, tag/search/editor behavior, terminal overstrikes, and I/O failures. Remaining boundaries are unavailable UTF-8 collation providers and terminal-capability/platform integration—not absent translated catalogs or unsupported `-i`, `-p`, or `-t`. |
| 2 | `cp` | `OUTPUT_FILES`, platform/error integration | Accepted source now rejects unsafe/aliased destinations, preserves physical symlink metadata without mutating referents, preserves portable atime or fails loudly, and has exact same-file/destination/umask/PATH_MAX tests. Remaining: privileged device-node and ownership products, injected mid-copy read/write/unlink failures, non-Linux/Darwin symlink metadata, and Windows runtime paths. |
| 2 | `touch` | `OUTPUT_FILES`, platform/error integration | Accepted source now obtains reference atime on supported stat layouts and fails loudly when unavailable; literal `-`, TZ, leap second, near-PATH_MAX, and reference-time paths are tested. Remaining: real atime propagation on every target, 0666 creation through umask, `-c` existing-file and multi-operand failure products, and full range rejection. |
| 2 | `iconv` | `ENVIRONMENT_VARIABLES`, locale-provider integration | Accepted source implements pathname charmaps, strict malformed/truncated detection across every carried multibyte family, `-c` status invariance, exact `-s` scope, locale-derived omitted encodings, aliases, file/stdin ordering, and read/write failures. Remaining: locale codeset discovery is a deterministic carried corpus rather than `nl_langinfo(CODESET)`; unknown unqualified installed locales fail closed. |
| 3 | `chgrp`, `chown`, `chmod`, `mkdir`, `mkfifo`, `mv`, `rm`, `uudecode`, `uuencode` | `OUTPUT_FILES`, `CONSEQUENCES_OF_ERRORS` | Add privilege-contained kernel/filesystem tests for ownership, mode/set-ID/ctime, symlink/hard-link identity, special files, permission failures, and platform-specific implementations rather than seam-only proof. |
| 3 | `df`, `du` | `STDOUT`, `CONSEQUENCES_OF_ERRORS` | Add mounted cross-device fixtures, real mount discovery, hard-link products, free-slot/space accounting, output failures, and platform-specific formats. |
| 3 | `logger` | `EFFECTS`, `EXIT_STATUS` | Add a real local syslog receiver and Windows disposition test; prove message persistence, zero-operand stdin behavior, open/send/close errors, and statuses. |
| 3 | `nohup` | `OUTPUT_FILES`, `EXIT_STATUS` | Test `nohup.out` mode through umask, append preservation, both output-open failures preventing execution, non-ENOENT start failures, and non-skipping PTY paths. |
| 3 | `ps` | `STDOUT`, `ENVIRONMENT_VARIABLES` | Add runtime providers for non-Linux targets and tests for every required field, identity lookup, unavailable-data representation, terminal width, locale time, and live selection semantics. |
| 3 | `newgrp` | `EFFECTS`, privilege/host integration | Accepted source proves name-before-numeric lookup, membership/password/TTY policy, supplementary capacity fallback, equal real/effective GID plans, unchanged-shell retry after refusal, login argv0/environment/cwd, virtual umask, streams, and shell status/signal propagation. Remaining: a real setuid-root credential transition, attended controlling-terminal password entry, NSS/PAM policy, and non-Unix disposition are host/privilege integration boundaries. |
| 3 | `pax` | `OUTPUT_FILES`, platform/filesystem integration | Accepted issues 715-717 now cover interactive rename, copy links, source-atime reset, ordered `-p`, preservation failures, the full `-o` grammar/precedence families, extended-header transcoding, complete list formats, locale time, and deterministic errors. Remaining: special-file extraction, privileged ownership/device products, unsupported no-follow metadata platforms, real terminal breadth, and uncarried legacy locale encodings. |
| 3 | `renice` | `EFFECTS`, `EXIT_STATUS` | Add a hermetic scheduler seam for exact `which` dispatch and mixed ID success, plus privilege-contained real priority-change tests and Windows disposition. |
| 3 | `stty`, `tabs`, `tput` | terminal `STDIN`/`STDOUT`, `EFFECTS` | Run required forms against real terminal/terminfo databases across supported platforms, including Windows disposition, unavailable capabilities, atomic state, write errors, and exact statuses. |
| 3 | `who`, `write` | `INPUT_FILES`, `OUTPUT_FILES`, terminal effects | Add live login-database and PTY fixtures across supported ABIs, credentials, terminal ownership/activity/permission state, interruption, framing, close errors, and platform fail-closed behavior. |
| 4 | `cat` | command-specific stream/status edge | Accepted tests now prove injected mid-stream stdin read failures through both the copy and line loops (diagnostic naming the operand, status 1, later operands still processed), directory and dangling-symlink operands with continuation, `/dev/null` alone and interleaved, and FIFO-through-symlink streaming. Remaining: unlocalized diagnostics in non-C locales (superseded product gap), Windows special-file behavior, and the process-level SIGPIPE/default broken-pipe disposition, which is not recorded as evidence (XCU cat ASYNCHRONOUS EVENTS is Default). |
| 4 | `basename`, `cksum`, `cmp`, `dirname`, `env`, `head`, `ln`, `mesg`, `rmdir`, `sleep`, `split`, `strings`, `tail`, `tee`, `tsort`, `uname`, `xargs` | command-specific stream/status edge | The closure audits find the main C/POSIX algorithm supportable, but each row still names concrete unproved I/O, signal, special-file, platform, or locale branches. Add exactly the missing observable test listed in its closure audit; no synthetic per-command catalog fixture is imposed. |

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
routing references. `false` and `true` are verified. The other twenty remain
partial because their closure audits identify concrete locale, interactive,
process, filesystem, or grammar residuals. The new `bg`, `fc`, `fg`, and `jobs`
references are
command-specific sibling-sh tests from `c354d6fc`; the `sh` row additionally
uses Bashy's process-level entrypoint contract alongside its two independent
route/strict-mode tests. Validator evidence is pinned to canonical sibling
`sh` at `6330c050` and `bashy` at `d9e1622`; retained workspace copies are not
authoritative evidence roots. No shell repository is modified by this
reconciliation. These path-and-test references do not prove complete clause
coverage: that is why twenty rows remain partial.

## Accepted source-wave reconciliation

This report is reconciled through canonical `94a4a7b`. It credits the accepted command
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
amendment `4c3d133`.

The machine-readable evidence lanes name the exact current test IDs, including
the nice pre-exec barrier/failure path, newgrp credential/login/umask helper,
pax interactive/preservation/extended-header families, iconv
charmap/stream/status matrix, fold locale/byte/error matrix, getconf
inventory/platform/error matrix, cut and expand locale/byte/error matrices,
paste locale-delimiter/error matrix, and cat injected-read/special-file
continuation evidence.
These recently accepted locale/platform waves remain partial because their
locale-provider and platform residuals are stated explicitly above.

## Reproduction

```sh
python3 scripts/posix_manifest.py --check
python3 scripts/posix_manifest.py --require-owned-complete
python3 -m unittest scripts/posix_manifest_test.py
scripts/applet-matrix.py --check
```

The owned-completion command is expected to fail at this snapshot with 97
items: 77 of 78 Go rows and 20 shell rows are partial. Equivalently, `nice` is
the sole verified Go row and 20 of 22 shell rows remain incomplete.
That failure is the truthful Sprint 79 residual, not a waived gate.
