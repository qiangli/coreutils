# Profile C/D POSIX interface audit: shell-selected commands

This document audits the 22 names whose **effective** owner in Profiles C/D is
the shell: `alias`, `bg`, `cd`, `command`, `echo`, `false`, `fc`, `fg`,
`getopts`, `hash`, `jobs`, `kill`, `printf`, `pwd`, `read`, `sh`, `test`,
`time`, `true`, `umask`, `unalias`, and `wait`.  The normative baseline is The
Open Group Base Specifications Issue 7, 2016 Edition.  It is deliberately a
command-specific companion to the generated interface ledger, not a change to
that ledger or its generator.

## Verdict vocabulary and evidence limits

- **verified** means the complete applicable Issue 7 interface is both present
  in the selected shell path and covered by focused public repository evidence.
- **implementation_gap** means source or retained Profile B results identify a
  behavior that still needs correction or attribution.  For this batch the fix
  belongs to the Bashy shell/job runtime, not a same-name Go applet.
- **evidence_gap** means the implementation is present and no current
  Bashy-only result blocker is known, but the public evidence does not prove the
  complete command interface.  Source presence, a Bash 5.3 differential, and an
  aggregate VSC result are not substitutes for clause-level evidence.

The Profile B record used here is the sanitized 2026-08-22 four-arm diagnostic
baseline: GNU Bash 5.3 versus Bashy, both with the same GNU/system utilities.
Its cloud lanes executed all 9,337 results, but their launchers were rejected
after execution; the local Bashy lane also capped in `fc` and omitted 51
results.  It is therefore useful differential evidence, **not certification
evidence**.  The record has three cross-platform Bashy-only blockers, none in
this 22-name batch.  It separately records `kill:TP9` as a shared A/B result
failure with a Bashy-only timing regression, `kill_NE:TP8` as shared, and the
ARM `fc` cap.  The 252 A=0/B=3 results in `echo_NE`, `false_NE`, `printf_NE`,
`pwd_NE`, `test_NE`, and `true_NE` are capability/`NOTINUSE` dispositions, not
behavioral passes and not blocker failures.

“Likely TP impact” uses the 2026-08-23 public-safe testability inventory
(`configured TPs - UNTESTED`, clamped to zero for the anomalous `jobs` count).
It ranks opportunity, not conformance: critical >= 50, high 25–49, medium
10–24, low < 10 testable TPs.

Two conditional markings below intentionally do not repeat the generated
ledger's current synopsis candidates.  On the cited 2016 pages the `[UP]`
margin encloses **all** `fc` forms, and the `[XSI]` margin encloses both
`kill [-signal_name]` and `kill [-signal_number]`.  Thus `fc` is conditional
as a whole and the base `kill` interface is only `-s`/`-l`; this audit records
the normative page exactly while leaving the shared canonical TSV/generator
untouched as required.

## Exact Profile C/D routing and selection contract

Profile C is stock GNU Bash 5.3 plus the staged Bashy Go userland.  Profile D
is Bashy 5.3 plus the same staged userland.  The certification entry is named
`sh`, and `POSIXLY_CORRECT` is set and exported by the staged environment.
Profile C must therefore be stock Bash 5.3's `sh`/POSIX route; Profile D must
be Bashy's `sh` route, not the AgentOS `bashy` front door and not merely a
non-POSIX Bashy runner.

For Profile D, `internal/cli/main.go` in the Bashy repository computes POSIX
startup from argv[0], applies `interp.Params("-o", "posix")`, and, only when
argv[0]'s basename is `sh` (also `-sh`), applies
`interp.WithStrictPosix(true)`.  The shared shell then:

1. expands aliases during parsing before command lookup;
2. selects shell functions before the non-special builtins in this batch;
3. invokes the intrinsic set `alias bg cd command fc fg getopts hash jobs kill
   read umask unalias wait` without requiring a PATH hit;
4. PATH-gates the regular builtins `echo false printf pwd test true` in strict
   `sh` mode, then executes the builtin rather than the found executable; and
5. parses `time` as a keyword/`TimeClause` when it prefixes a command or
   pipeline (`time` and `time -p` alone retain Bash's special bare form).

The source anchors are Bashy's `internal/cli/main.go` (`newRunner`,
`invokedAsSh`), the sibling shell's
[`interp/runner.go`](../../../sh/interp/runner.go) (`isStrictPosixIntrinsic`,
`Runner.call`), [`interp/builtin.go`](../../../sh/interp/builtin.go)
(`IsBuiltin`, `Runner.builtin`), and
[`syntax/parser.go`](../../../sh/syntax/parser.go) (`timeClause`).  The stock
Bash 5.3 comparison is `builtins/builtins.c` plus the corresponding `.def`
files, and `parse.y`/`execute_cmd.c` for `time`.

The required effective classifications are consequently: `sh` = file,
`time` = keyword, and every other name in this document = builtin.  Seven of
those builtins (`echo`, `false`, `kill`, `printf`, `pwd`, `test`, `true`) also
have staged Go applets; `time` also has a Go applet.  Those PATH entries serve
exec-style callers such as `env`, `xargs`, or `find -exec`.  They do not own a
direct shell invocation.  A direct-shell defect must be fixed and evidenced in
Bash/Bashy shell code; duplicating the change in `cmds/<name>` cannot close it.

## Priority summary

| Rank | Command | Status | Likely testable TPs | Why it is not stronger |
| ---: | --- | --- | ---: | --- |
| 1 | `sh` | evidence_gap | 245 (critical) | Broad shell set; no clause-complete public command evidence. |
| 2 | `test` | evidence_gap | 207 (critical) | Large expression grammar; shell lane is still `UNVERIFIED`. |
| 3 | `printf` | evidence_gap | 67 (critical) | Format, locale, diagnostics, and reuse rules lack shell-specific closure. |
| 4 | `cd` | evidence_gap | 45 (high) | Prior targeted Profile B gaps were retired, not transcribed as full interface evidence. |
| 5 | `command` | evidence_gap | 37 (high) | Lookup/query semantics and `-p` need focused shell evidence. |
| 6 | `fc` | evidence_gap | 28 (high) | Cloud completed; ARM capped and omitted the whole 51-result set. |
| 7 | `umask` | evidence_gap | 27 (high) | Numeric/symbolic tests exist, but the Issue 7 interface is not closed. |
| 8 | `kill` | implementation_gap | 18 (medium) | Exact shared `TP9` failure remains; Bashy additionally has the measured timing regression. |
| 9 | `bg` | evidence_gap | 17 (medium) | Conditional job-control utility; PTY/job-state evidence incomplete. |
| 10 | `pwd` | evidence_gap | 17 (medium) | Prior five-identity closure is targeted rather than clause-complete. |
| 11 | `time` | evidence_gap | 16 (medium) | Keyword route exists; full POSIX output/status evidence is absent. |
| 12 | `read` | evidence_gap | 13 (medium) | Public reducers cover field/backslash cases, not the complete interface. |
| 13 | `alias` | evidence_gap | 13 (medium) | Broken-pipe TP was fixed; listing/substitution interface remains unclosed. |
| 14 | `wait` | evidence_gap | 13 (medium) | PID/job lifecycle tests exist; complete status semantics are unproven. |
| 15 | `echo` | evidence_gap | 12 (medium) | `_NE` dispositions are not positive command evidence. |
| 16 | `fg` | evidence_gap | 12 (medium) | Conditional interactive/job-control surface lacks complete PTY evidence. |
| 17 | `hash` | evidence_gap | 12 (medium) | Prior three-identity closure plus unit tests do not cover every Issue 7 effect. |
| 18 | `getopts` | evidence_gap | 10 (medium) | TP6 was fixed; OPTIND/OPTARG/error-mode matrix remains incomplete. |
| 19 | `unalias` | evidence_gap | 8 (low) | Prior TP4 retired without a clause-complete evidence record. |
| 20 | `false` | verified | 7 (low) | Entire interface is no operands/options, status >0; source and focused tests cover it. |
| 21 | `true` | verified | 6 (low) | Entire interface is no operands/options, status 0; source and focused tests cover it. |
| 22 | `jobs` | evidence_gap | 0 (low) | Suite yield is zero, but the conditional standard interface still exists. |

## Command audits

### `alias` — evidence_gap, medium (13)

Normative interface: [`alias [alias-name[=string]...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/alias.html), no options; operands query or define aliases, and no operands lists all aliases in reusable form.  Bash 5.3 uses `builtins/alias.def`; Bashy uses `builtin.go`'s `alias` case and its parser-time alias table.  Profile B previously exposed listing write failure (`alias:TP20`); shell commit `d4fe7f7e` and `interp` broken-pipe tests address that point.  The public ledger still has no `sh:` evidence ID for the full substitution, quoting, listing, diagnostics, and status contract.

### `bg` — evidence_gap, medium (17)

Normative conditional interface: [`bg [job_id...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bg.html), required only when the User Portability Utilities option is supported; no options.  It resumes each selected stopped job in the background and reports failure for unavailable/invalid job IDs or when job control is disabled.  Bash 5.3 uses `builtins/fg_bg.def`; Bashy uses `builtin.go` plus the shared job table/carrier and has POSIX job-line tests.  There is no complete interactive PTY/job-state evidence lane.

### `cd` — evidence_gap, high (45)

Normative interface: [`cd [-L|-P] [directory]` and `cd -`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html); `-L` and `-P` are required, `-` selects `OLDPWD`, and HOME/CDPATH/PWD/OLDPWD plus logical component processing and required pathname output are observable effects.  Bash 5.3 uses `builtins/cd.def`; Bashy uses `builtin.go` with focused `TestCdPosixComponent` and `TestCdPwdPosixSymlinkSemantics`.  Sprint 68 retired four Profile B identities in complete targeted pairs, but that does not prove every normative branch.

### `command` — evidence_gap, high (37)

Normative interface: [`command [-p] command_name [argument...]` and `command [-p][-v|-V] command_name`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html); required options are `-p`, `-v`, and `-V`.  Execution suppresses function lookup for the named command, `-p` supplies a standard utility PATH, and query forms describe resolution with specified output/status behavior.  Bash 5.3 uses `builtins/command.def` and `execute_cmd.c`; Bashy uses the `command` case in `builtin.go` and nested `Runner.call`/exec routing.  The historical TP23/27 work is not retained here as complete lookup/query evidence.

### `echo` — evidence_gap, medium (12)

Normative base interface: [`echo [string...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/echo.html), no options, and `--` is an operand.  If the first operand is `-n` or any operand contains `\\`, base POSIX makes results implementation-defined; XSI additionally defines `\\a \\b \\c \\f \\n \\r \\t \\v \\\\ \\0num`.  Bash 5.3 uses `builtins/echo.def`; Bashy uses `builtin.go`'s `echo` case.  The `_NE` capability results do not establish either the base-defined region or the conditional XSI escapes, and no command-complete shell evidence is recorded.

### `false` — verified, low (7)

Normative interface: [`false`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/false.html), no options or operands, no output, exit status greater than zero.  Bash 5.3 uses `builtins/colon.def`; Bashy dispatches `false` directly to status 1 in `builtin.go`.  Focused interpreter cases cover bare and extra-argument extension behavior and status propagation.  The staged Go `false` remains necessary for exec-style use, but direct Profile C/D shell calls select the builtin.

### `fc` — evidence_gap, high (28)

Normative conditional interface: the User Portability Utilities option adds all three forms, [`fc [-r] [-e editor] [first [last]]`, `fc -l [-nr] [first [last]]`, and `fc -s [old=new] [first]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fc.html), with `-e editor`, `-l`, `-n`, `-r`, and `-s`.  There is no base `fc` synopsis outside the `[UP]` margin.  FCEDIT/HISTFILE/HISTSIZE and history-number/string selection are observable.  Bash 5.3 uses `builtins/fc.def`; Bashy uses `builtin.go` plus `history.go`, with focused editor/range/listing tests.  The cloud Profile B set completed, but the local ARM lane capped after 600 seconds and lost all 51 results, so a portable complete evidence claim is unavailable.

### `fg` — evidence_gap, medium (12)

Normative conditional interface: [`fg [job_id]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fg.html), required only with User Portability Utilities; no options.  It foregrounds the selected/current job, continues it if stopped, waits, and returns its status.  Bash 5.3 uses `builtins/fg_bg.def`; Bashy uses the shared builtin/job carrier.  Source and non-interactive unit coverage do not close controlling-terminal and interactive selection behavior.

### `getopts` — evidence_gap, medium (10)

Normative interface: [`getopts optstring name [arg...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html), no utility options; `optstring`, `name`, and optional argument vector drive OPTIND/OPTARG and normal versus leading-colon error reporting.  Bash 5.3 uses `builtins/getopts.def`; Bashy uses `builtin.go`'s `getopts` state machine.  Profile B `TP6` found acceptance of `:` as an option character and was fixed by `87f45fee`; the remaining reset, repeated-call, explicit-arg, diagnostics, and exit matrix lacks one complete public evidence ID.

### `hash` — evidence_gap, medium (12)

Normative interface: [`hash [utility...]` and `hash -r`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html); `-r` is required, operands update/report remembered utility locations, and no operands may report the table.  Bash 5.3 uses `builtins/hash.def`; Bashy uses `builtin.go` and `cmdHashTable`, with dedicated `hash_builtin_test.go`.  Sprint 68 retired three targeted Profile B identities in repeated complete pairs, but the generated ledger correctly remains unverified because no clause-complete shell evidence reference was recorded.

### `jobs` — evidence_gap, low (0)

Normative conditional interface: [`jobs [-l|-p] [job_id...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/jobs.html), required only with User Portability Utilities; `-l` and `-p` are conditional required options and output is tied to the shell's current jobs.  Bash 5.3 uses `builtins/jobs.def`; Bashy uses `builtin.go` and the job formatter.  The inventory counts five UNTESTED markers against four configured TPs, yielding no likely earnable TP, but that measurement oddity does not waive the interface or prove it.

### `kill` — implementation_gap, medium (18)

Normative base forms: [`kill -s signal_name pid...` and `kill -l [exit_status]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/kill.html), with required `-s` and `-l`.  XSI conditionally adds both `kill [-signal_name] pid...` and `kill [-signal_number] pid...`; neither shorthand is in the base synopsis.  PID operands include the POSIX process/group meanings and shell job IDs are required for the builtin.  Bash 5.3 uses `builtins/kill.def` plus jobs/signal code; Bashy uses `builtin.go`, `kill_unix.go`, and the carrier.  The latest four-arm record has `kill:TP9` failing in all A/B lanes and `kill_NE:TP8` shared; Bashy also takes about 428–431 seconds where Bash takes about 172.  Candidate `54c05236` reduced the targeted two-activity Bashy time near the control but was not part of the audited tree.  Until the shared result failure is attributed and the Bashy runtime fix is integrated/re-measured, this is an implementation gap—not a reason to patch `cmds/kill`.

### `printf` — evidence_gap, critical (67)

Normative interface: [`printf format [argument...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/printf.html), no options.  Required behavior includes the specified ordinary conversions, `b` operand escapes including `\\c`, format reuse, missing/excess arguments, LC_NUMERIC, output errors, and non-zero error status.  Bash 5.3 uses `builtins/printf.def`; Bashy uses `builtin.go`'s independent formatter.  Go-applet numeric residual tests do not evidence this selected builtin, and Profile B `_NE` dispositions are not positive coverage.

### `pwd` — evidence_gap, medium (17)

Normative interface: [`pwd [-L|-P]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html), with required mutually exclusive `-L` and `-P`, logical PWD validity rules, physical fallback, absolute output, diagnostics, and status.  Bash 5.3 shares `builtins/cd.def`; Bashy uses the `pwd` case plus `validLogicalPWD`, with symlink and over-PATH_MAX tests.  Sprint 68 retired five targeted Profile B identities, but `_NE` results and Go-applet tests cannot close the builtin lane.

### `read` — evidence_gap, medium (13)

Normative interface: [`read [-r] var...`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/read.html); `-r` is required.  It reads one logical line, applies backslash continuation unless `-r`, splits by IFS, assigns the remainder to the last variable, affects the current shell, and distinguishes EOF/error status.  Bash 5.3 uses `builtins/read.def`; Bashy uses the `read` case and signal-aware input code.  Public Austin reducers cover remainder, backslash, and raw cases and signal tests cover interruption, but the complete stdin/IFS/assignment/error interface is not consolidated.

### `sh` — evidence_gap, critical (245)

Normative interfaces: [`sh` command-file, `-c` command-string, and `-s` stdin forms](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sh.html), with required `-a -b -C -e -f -h -i -m -n -u -v -x -o`, matching `+` forms, plus `-c` and `-s`; `-o/+o` take an option name.  The interface also owns shell grammar, expansion, redirection, execution, environment/startup, traps, jobs, and exit semantics.  Profile D routing is the argv0=`sh` strict route described above; Profile C is Bash 5.3 invoked as `sh`.  `sh_08:TP1` passes all four latest arms and Sprint 68 retired eight selected `sh_01`/`sh_03` identities, but neither fact proves the 245-testable-TP command as a whole.  The canonical shell evidence lane is empty, so the only defensible classification is evidence gap.

### `test` — evidence_gap, critical (207)

Normative interfaces: [`test [expression]` and `[ [expression] ]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/test.html), no utility options.  Base requirements include the zero-through-four-argument rules, pathname/string/integer primaries, and `!`; the obsolescent `-a`/`-o` forms and parentheses are XSI-conditional.  `--` is expression data, not an option delimiter.  Bash 5.3 uses `builtins/test.def`; Bashy uses `builtin.go`/`test.go` and parser support for `[`.  The Go applet's tests are not evidence for direct shell selection, while the Profile B `_NE` disposition is only capability routing.

### `time` — evidence_gap, medium (16)

Normative interface: [`time [-p] utility [argument...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/time.html); `-p` is required, PATH selects the utility, timing output goes to standard error in the required portable format, and the result normally follows the utility status.  Bash 5.3 owns this in `parse.y`/`execute_cmd.c` (`CMD_TIME_POSIX`); Bashy owns it in `syntax.TimeClause` and runner timing code.  The parser and formatting tests prove selected cases, but no full shell evidence covers keyword ambiguity, utility-not-found/status, signal, locale, and output requirements.

### `true` — verified, low (6)

Normative interface: [`true`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/true.html), no options or operands, no output, exit status zero.  Bash 5.3 uses `builtins/colon.def`; Bashy dispatches `true` as a no-op success in `builtin.go`.  Focused interpreter cases cover bare and extra-argument extension behavior and status propagation.  As with `false`, the staged Go applet backs exec-style calls only.

### `umask` — evidence_gap, high (27)

Normative interface: [`umask [-S] [mask]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/umask.html); `-S` is required, `mask` accepts symbolic or octal forms, no operand reports the mask, and setting it must affect the current shell and children.  Bash 5.3 uses `builtins/umask.def`; Bashy uses `builtin.go` and has dedicated symbolic/numeric/round-trip tests.  The prior `TP6/TP26` Profile B work and current tests are substantial but are not a retained command-complete evidence record.

### `unalias` — evidence_gap, low (8)

Normative interfaces: [`unalias alias-name...` and `unalias -a`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unalias.html); `-a` is required, and removal affects the current shell's subsequent parsing.  Bash 5.3 shares `builtins/alias.def`; Bashy uses `builtin.go`'s alias table.  Historical `TP4` is retired in matched current controls without a code change, but the full missing-name, `-a`, parse-timing, diagnostics, and status surface has no focused shell evidence ID.

### `wait` — evidence_gap, medium (13)

Normative interface: [`wait [pid...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wait.html), no options.  Operands may denote known asynchronous PIDs (and builtin job IDs); no operands waits for all known jobs, statuses must be retained, unknown IDs return 127, and signal termination produces a value greater than 128 distinguishable from normal exit status.  Bash 5.3 uses `builtins/wait.def` plus `jobs.c`; Bashy uses `builtin.go` plus the job carrier and has PID/status/reaping tests.  Those tests do not yet close all retention, multiple-operand, signal, subshell, and interactive cases.

## Required next evidence

The highest-value next batch is shell-owned, not Go-applet work: add stable
`sh:<path>#<TestID>` evidence references for `sh`, `test`, `printf`, `cd`, and
`command`; integrate and re-measure the `kill` job-carrier correction; and run
an uncapped ARM `fc` control/candidate pair.  Only after those focused results
cover stdout, stderr, status, environment and shell effects should the
canonical evidence ledger be updated by its owner.  This audit intentionally
does not edit that TSV or generator.
