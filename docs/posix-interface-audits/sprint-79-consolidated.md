# Sprint 79: POSIX required-command interface status

This report reconciles the Sprint 79 interface ledger at coreutils `bf800b4`
against POSIX.1-2016 Issue 7, current source, command-package tests, and the
sibling `sh` and `bashy` evidence repositories. The canonical machine-readable
source is [`posix-required-command-interfaces.tsv`](../posix-required-command-interfaces.tsv).
GNU extension behavior is not certification evidence.

## Exact denominator and current verdict

The ledger contains exactly 116 required names: 78 effective Go-owned, 22
effective shell-owned, and 16 external-provider-owned. The current evidence
states are **2 verified, 93 partial, and 21 unverified**.

| Effective owner | Verified | Partial | Unverified | Total |
| --- | ---: | ---: | ---: | ---: |
| Go | 0 | 78 | 0 | 78 |
| Shell | 2 | 15 | 5 | 22 |
| External provider | 0 | 0 | 16 | 16 |
| Total | 2 | 93 | 21 | 116 |

The only verified owned interfaces are shell-selected `false` and `true`.
Their status-only interfaces have both command-specific semantic evidence in
`sh` and command-specific Profile B routing evidence in `bashy`. No Go row is
promoted merely because its package suite is dense.

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

All 78 Go rows remain partial for at least this exact common blocker:

| Commands | Missing Issue 7 clauses | Product behavior and focused test required before verification |
| --- | --- | --- |
| all 78 Go-owned names above | `ENVIRONMENT_VARIABLES`, and the command's diagnostic obligations in `STDERR`/`CONSEQUENCES_OF_ERRORS` | Resolve `LC_ALL` over `LC_MESSAGES`, use `NLSPATH` where XSI applies, select a real message catalog, and emit a translated command-specific diagnostic without mutating process-global locale state. Add a package-local `Test<Command>LCMessagesCatalogPrecedence` that installs at least two distinguishable catalogs, proves `LC_ALL` precedence, proves `NLSPATH` selection, and asserts stderr plus non-zero status. Commands with no success-path diagnostic must drive a real required error branch. |

The repository has bounded locale providers for selected commands, but no
general message-catalog implementation proving that obligation for every Go
utility. Therefore even rows whose C/POSIX algorithms are otherwise closed
cannot truthfully be verified yet.

## Ranked command-specific residuals

The table records the highest-yield blocker beyond the universal catalog test.
The accepted per-command closure audits remain authoritative for additional
lower-ranked edges.

| Rank | Command(s) | Missing clause | Concrete product behavior and focused test needed |
| ---: | --- | --- | --- |
| 0 | `pax` | `OPTIONS`, `STDIN`, `STDOUT`, `OUTPUT_FILES`, `EXIT_STATUS` | Current pin still rejects every `-o` keyword invocation. Merge the separately reviewed `pax -o` source wave, then add named end-to-end tests for each supported Issue 7 keyword family, ordered repeated `-o`, archive/list effects, diagnostics, and status. Do not credit the active wave before its canonical SHA is present. |
| 0 | `cp` | `OPERANDS`, `STDIN`, `OUTPUT_FILES`, `CONSEQUENCES_OF_ERRORS` | Await the active cp correction wave. Re-run exact same-file ordering, physical symlink overwrite identity, `-i` response routing, recursive umask, `-p` set-ID failure, device/special-file, injected read/write/unlink failure, and continuation tests after the canonical merge. |
| 0 | `nice` | `EFFECTS`, `EXIT_STATUS` | Await the active nice correction wave. The child must not execute before priority adjustment. A process-boundary barrier test must prove ordering, adjustment failure prevents execution, and 126/127/child/signal statuses remain exact. |
| 0 | `touch` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Await the active touch correction wave. Re-prove literal `-`, `-r` access-time propagation on every platform path, 0666 creation through umask, existing-file `-c`, multi-operand continuation, timestamp ranges, and `TZ` before crediting it. |
| 0 | `newgrp` | `STDIN`, `ENVIRONMENT_VARIABLES`, `EFFECTS`, `EXIT_STATUS` | Await the active newgrp correction wave. A privilege-contained integration test must prove password input routing, real/supplementary group state, login environment, shell replacement-equivalent behavior, refusal behavior, and propagated shell status. |
| 1 | `bg`, `fc`, `fg`, `jobs`, `sh` | all interface clauses; semantic evidence lane absent | Add one command-specific sibling-sh test per command that exercises every syntax, option argument, operand, environment, stream, state effect, and status path. Existing Bashy Profile B route tests prove selection only and cannot substitute. |
| 2 | `at`, `batch`, `crontab` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Add scheduler integration tests for access policy, delivery/mail behavior, load gating, all locale time grammars, daemon handoff, and persisted execution environment. |
| 2 | `awk`, `comm`, `csplit`, `cut`, `expand`, `expr`, `fold`, `grep`, `join`, `sed`, `sort`, `tr`, `unexpand`, `uniq`, `wc` | `ENVIRONMENT_VARIABLES`, algorithmic `STDOUT` | Add non-C multibyte `LC_CTYPE`/`LC_COLLATE`/`LC_NUMERIC` fixtures that discriminate character boundaries, classes, equivalence, ranges, ordering, blanks, widths, and numeric rendering for each named command. |
| 2 | `date` | XSI `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Add privileged clock-set integration coverage, leap-second rendering, additional platform setters, and a complete installed-locale `LC_TIME` matrix with mutation-after-validation checks. |
| 2 | `getconf` | `OPERANDS`, `STDOUT` | Add a generated exhaustive test for every required sysconf/pathconf/confstr/minimum name, pathname-vs-system routing, undefined/indeterminate output, specification operands, and status. |
| 2 | `iconv` | `OPTIONS`, `ENVIRONMENT_VARIABLES`, `EXIT_STATUS` | Implement charmap-file operands and strict invalid-sequence detection so `-c` never changes status; test omitted encodings from locale, read errors under `-s`, every accepted encoding name, and malformed/truncated sequences. |
| 2 | `file` | `OPTIONS`, `STDOUT` | Add complete magic-file grammar and required type-string tests, including `-d`, `-i`, `-M`, symlink policy, stdin, inaccessible operands, and locale effects. |
| 2 | `find` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `EFFECTS` | Reject the omitted-path extension in POSIX mode or document a gated route; add all primary/action products, real ownership databases, locale `-ok`/pattern behavior, filesystem failures, and `-exec` side effects. |
| 2 | `id` | `STDOUT` | Add named-user default/group output, lookup-failure fallbacks, a real set-ID process fixture, and executable non-Unix behavior or an explicit platform conformance disposition. |
| 2 | `logname` | `STDOUT`, `EXIT_STATUS` | Add a real login-session fixture on each supported platform; prove success without environment fallback and failure only when no login identity exists. |
| 2 | `more` | `OPTIONS`, `ENVIRONMENT_VARIABLES`, interactive effects | Complete terminal `-i`, `-p`, conditional `-t`, `$MORE`, locale command input, editor handoff, and the full interactive grammar with PTY tests. |
| 2 | `od` | `ENVIRONMENT_VARIABLES`, `STDOUT` | Add a non-C `LC_CTYPE` `-c` rendering fixture and non-C `LC_NUMERIC` floating-format fixture across all required type strings and ABI sizes. |
| 2 | `paste` | `OPERANDS`, `ENVIRONMENT_VARIABLES`, `STDOUT` | Add locale-aware delimiter decoding, repeated `-` under `-s`, twelve-operand coverage, serial `\\0`, input read errors, and stdout failure. |
| 2 | `pathchk` | `OPERANDS`, `EXIT_STATUS` | Query actual containing-filesystem limits and validate pathname byte sequences; test differing mount limits, missing prefixes, symlinks, search permission, and invalid encodings. |
| 2 | `pr` | `OPTIONS`, `ENVIRONMENT_VARIABLES`, `STDOUT` | Add exact Issue 7 optional-argument grammar, every column/merge/page interaction, locale date/header width, terminal pause/interruption, input/output failure, and status matrix. |
| 2 | `tty` | `STDOUT`, `EXIT_STATUS` | Implement truthful terminal-name discovery on every supported POSIX target and locale-sensitive nonterminal output; add real PTY/non-PTY tests per platform. |
| 3 | `chgrp`, `chown`, `chmod`, `mkdir`, `mkfifo`, `mv`, `rm`, `uudecode`, `uuencode` | `OUTPUT_FILES`, `CONSEQUENCES_OF_ERRORS` | Add privilege-contained kernel/filesystem tests for ownership, mode/set-ID/ctime, symlink/hard-link identity, special files, permission failures, and platform-specific implementations rather than seam-only proof. |
| 3 | `df`, `du` | `STDOUT`, `CONSEQUENCES_OF_ERRORS` | Add mounted cross-device fixtures, real mount discovery, hard-link products, free-slot/space accounting, output failures, and platform-specific formats. |
| 3 | `logger` | `EFFECTS`, `EXIT_STATUS` | Add a real local syslog receiver and Windows disposition test; prove message persistence, zero-operand stdin behavior, open/send/close errors, and statuses. |
| 3 | `nohup` | `OUTPUT_FILES`, `EXIT_STATUS` | Test `nohup.out` mode through umask, append preservation, both output-open failures preventing execution, non-ENOENT start failures, and non-skipping PTY paths. |
| 3 | `ps` | `STDOUT`, `ENVIRONMENT_VARIABLES` | Add runtime providers for non-Linux targets and tests for every required field, identity lookup, unavailable-data representation, terminal width, locale time, and live selection semantics. |
| 3 | `renice` | `EFFECTS`, `EXIT_STATUS` | Add a hermetic scheduler seam for exact `which` dispatch and mixed ID success, plus privilege-contained real priority-change tests and Windows disposition. |
| 3 | `stty`, `tabs`, `tput` | terminal `STDIN`/`STDOUT`, `EFFECTS` | Run required forms against real terminal/terminfo databases across supported platforms, including Windows disposition, unavailable capabilities, atomic state, write errors, and exact statuses. |
| 3 | `who`, `write` | `INPUT_FILES`, `OUTPUT_FILES`, terminal effects | Add live login-database and PTY fixtures across supported ABIs, credentials, terminal ownership/activity/permission state, interruption, framing, close errors, and platform fail-closed behavior. |
| 4 | `basename`, `cat`, `cksum`, `cmp`, `dirname`, `env`, `head`, `ln`, `mesg`, `rmdir`, `sleep`, `split`, `strings`, `tail`, `tee`, `tsort`, `uname`, `xargs` | command-specific stream/status edge plus universal catalog blocker | The closure audits find the main C/POSIX algorithm supportable, but each row still names concrete unproved I/O, signal, special-file, platform, or locale branches. Add exactly the missing test listed in its closure audit, then the command-specific catalog-precedence test above. |

Detailed residual wording is in the accepted `go-evidence-closure-batch-1.md`
through `go-evidence-closure-batch-7c.md` reports.

## Shell-selected routing and evidence

Profile C selects GNU Bash invoked as `sh`; Profile D selects Bashy's
argv0=`sh` strict POSIX route. The same-name Go applets are therefore not the
direct shell owner for `echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`time`, and `true`.

Seventeen shell names have stable semantic and routing references. `false`
and `true` are verified. The other fifteen remain partial because their
closure audits identify concrete locale, interactive, process, filesystem,
or grammar residuals. `bg`, `fc`, `fg`, `jobs`, and `sh` have command-specific
routing tests but no command-specific semantic reference and remain
unverified. No shell repository is modified by this reconciliation.

## Active source waves are not credited

This report is deliberately pinned to `bf800b4`. The active `pax -o`, `cp`,
`nice`, `touch`, and `newgrp` waves are not present at this pin and are not
treated as implementation or evidence. After root supplies canonical merged
SHAs, each affected row must be re-audited against the merged source and exact
tests before its residual can change.

## Reproduction

```sh
python3 scripts/posix_manifest.py --check
python3 scripts/posix_manifest.py --require-owned-complete
python3 -m unittest scripts/posix_manifest_test.py
scripts/applet-matrix.py --check
```

The owned-completion command is expected to fail at this snapshot with 98
items: all 78 Go rows are partial, 15 shell rows are partial, and five shell
rows are unverified. Equivalently, all 78 Go rows and 20 of 22 shell rows
remain incomplete.
That failure is the truthful Sprint 79 residual, not a waived gate.
