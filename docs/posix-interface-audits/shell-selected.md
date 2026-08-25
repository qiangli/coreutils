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
  Promotion to verified requires a stable semantic `sh:<path>#<TestID>`
  reference, a distinct command-specific Bashy selection reference in the
  `bashy:<approved-path>#<TestID>` routing lane, **and** a separately-made
  promotion in the canonical ledger.  Routing can prove that the shell wins
  over a same-name Go applet, but it cannot substitute for command behavior;
  source presence, focused Go-applet tests, and an aggregate differential
  result are not sufficient.  `false` and `true` now meet that bar; the other
  20 commands remain partial or unverified.
- **implementation_gap** means source or retained Profile B results identify a
  Bashy-only behavior that still needs a code correction in the Bashy shell/job
  runtime (not a same-name Go applet).  A defect that fails **identically** in
  both A and B lanes is not a Bashy implementation gap; it is a shared-result
  gap recorded under evidence_gap below.
- **evidence_gap** means the implementation is present and no current
  Bashy-only result blocker is known, but the public evidence does not prove the
  complete command interface.  This also covers **shared-result gaps** — an A/B
  differential failure that reproduces in both lanes and therefore attributes to
  the shared suite/environment rather than a Bashy defect.  Source presence, a
  Bash 5.3 differential, and an aggregate VSC result are not substitutes for
  clause-level evidence.

The current Profile B authority is the **accepted 9,337/9,337 remote pair**:
GNU Bash 5.3 versus Bashy, both with the same GNU/system utilities, both arms
completing all 9,337 configured results with accepted launchers.  This is the
authoritative differential record for this audit.  It remains useful
differential evidence, **not certification evidence** — a complete A/B result
set still does not supply the clause-level command evidence promotion requires.
The record has three cross-platform Bashy-only blockers, none in this 22-name
batch.  It separately records `kill:TP9` as a **shared A/B result failure**
(fails identically in both lanes) and `kill_NE:TP8` as shared; because the
Bashy timing fix is now integrated (see the `kill` audit), the earlier
Bashy-only timing regression is closed and only the shared-result failure
remains.

The superseded 2026-08-22 four-arm diagnostic baseline is retained **only as
history**: its cloud lanes executed all 9,337 results but their launchers were
rejected after execution, and its local Bashy lane capped in `fc`.  It is no
longer the authority; the targeted ARM `fc` control/candidate pair has since
completed cleanly at **53/53** (recorded below), so the old cap is closed and
does not warrant a rerun.

The 252 A=0/B=3 `_NE` results are capability/`NOTINUSE` dispositions, not
behavioral passes and not blocker failures.  They span **seven** sets —
`echo_NE`, `false_NE`, `leftbrack_NE`, `printf_NE`, `pwd_NE`, `test_NE`, and
`true_NE` — where `leftbrack_NE` is the `[` form of `test` and was omitted from
the earlier six-set accounting.

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

The canonical ledger represents those two proof obligations independently.
`shell_evidence` accepts only focused semantic tests from the sibling shell as
`sh:<path>#<TestID>`.  `shell_routing_evidence` accepts only command-specific
integration tests from the sibling Bashy repository as
`bashy:internal/cli/<file>_test.go#<TestID>`.  AgentOS tests are deliberately
excluded because the AgentOS front door is not Profile D's `sh` route.  The
routing lane is legal only when `effective_owner=shell`; a verified shell row
must have valid references in both lanes.  Neither lane may stand in for the
other.  All 22 routing cells now name the command-specific tests accepted in
Bashy commit `d851773`; this closes only the selection proof, so semantic
evidence and evidence states remain unchanged.

**`sh` entrypoint reconciliation.**  The canonical interface ledger now records
`sh` with `parser_model = shell_entrypoint` (commit `fe6f45d`), rather than the
earlier incorrect `shell_builtin` classification.  This matches the actual
file entrypoint that argv0=`sh` (`shell_only`, no Go package) resolves and
executes.  `time` remains correctly classified as `shell_keyword`.

## Priority summary

| Rank | Command | Status | Likely testable TPs | Why it is not stronger |
| ---: | --- | --- | ---: | --- |
| 1 | `sh` | evidence_gap | 245 (critical) | Broad shell set; no clause-complete public command evidence. |
| 2 | `test` | evidence_gap | 207 (critical) | Large expression grammar; shell lane is still `UNVERIFIED`. |
| 3 | `printf` | evidence_gap | 67 (critical) | Format, locale, diagnostics, and reuse rules lack shell-specific closure. |
| 4 | `cd` | evidence_gap | 45 (high) | Prior targeted Profile B gaps were retired, not transcribed as full interface evidence. |
| 5 | `command` | evidence_gap | 37 (high) | Lookup/query semantics and `-p` need focused shell evidence. |
| 6 | `fc` | evidence_gap | 28 (high) | Cloud complete and targeted ARM pair complete (53/53); clause-complete interface evidence still absent. |
| 7 | `umask` | evidence_gap | 27 (high) | Numeric/symbolic tests exist, but the Issue 7 interface is not closed. |
| 8 | `kill` | evidence_gap | 18 (medium) | `TP9` is a shared A/B failure (not a Bashy defect); Bashy timing fix already integrated (`031d47e2`), so no reintegration is owed. |
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
| 19 | `unalias` | evidence_gap | 8 (low) | Focused semantic and routing evidence now exists, but locale and full parse-timing coverage remain open; ledger state is partial. |
| 20 | `false` | verified | 7 (low) | Complete no-option/no-operand/status-only interface has focused semantic and routing evidence. |
| 21 | `true` | verified | 6 (low) | Complete no-option/no-operand/status-only interface has focused semantic and routing evidence. |
| 22 | `jobs` | evidence_gap | 0 (low) | Suite yield is zero, but the conditional standard interface still exists. |

## Command audits

### `alias` — evidence_gap, medium (13)

Normative interface: [`alias [alias-name[=string]...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/alias.html), no options; operands query or define aliases, and no operands lists all aliases in reusable form.  Bash 5.3 uses `builtins/alias.def`; Bashy uses `builtin.go`'s `alias` case and its parser-time alias table.  Profile B previously exposed listing write failure (`alias:TP20`); shell commit `d4fe7f7e` and `interp` broken-pipe tests address that point.  `TestAliasIssue7Interface` now covers definition, replacement, query/list quoting and order, trailing-blank substitution, subshell/invocation scope, missing-name continuation, write failure, and stdin preservation.  The ledger is conservatively partial until locale-sensitive diagnostics and every parser-timing boundary are closed.

### `bg` — evidence_gap, medium (17)

Normative conditional interface: [`bg [job_id...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bg.html), required only when the User Portability Utilities option is supported; no options.  It resumes each selected stopped job in the background and reports failure for unavailable/invalid job IDs or when job control is disabled.  Bash 5.3 uses `builtins/fg_bg.def`; Bashy uses `builtin.go` plus the shared job table/carrier and has POSIX job-line tests.  There is no complete interactive PTY/job-state evidence lane.

### `cd` — evidence_gap, high (45)

Normative interface: [`cd [-L|-P] [directory]` and `cd -`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html); `-L` and `-P` are required, `-` selects `OLDPWD`, and HOME/CDPATH/PWD/OLDPWD plus logical component processing and required pathname output are observable effects.  Bash 5.3 uses `builtins/cd.def`; Bashy uses `builtin.go` with focused `TestCdPosixComponent` and `TestCdPwdPosixSymlinkSemantics`.  Sprint 68 retired four Profile B identities in complete targeted pairs, but that does not prove every normative branch.

### `command` — evidence_gap, high (37)

Normative interface: [`command [-p] command_name [argument...]` and `command [-p][-v|-V] command_name`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html); required options are `-p`, `-v`, and `-V`.  Execution suppresses function lookup for the named command, `-p` supplies a standard utility PATH, and query forms describe resolution with specified output/status behavior.  Bash 5.3 uses `builtins/command.def` and `execute_cmd.c`; Bashy uses the `command` case in `builtin.go` and nested `Runner.call`/exec routing.  The historical TP23/27 work is not retained here as complete lookup/query evidence.

### `echo` — evidence_gap, medium (12)

Normative base interface: [`echo [string...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/echo.html), no options, and `--` is an operand.  If the first operand is `-n` or any operand contains `\\`, base POSIX makes results implementation-defined; XSI additionally defines `\\a \\b \\c \\f \\n \\r \\t \\v \\\\ \\0num`.  Bash 5.3 uses `builtins/echo.def`; Bashy uses `builtin.go`'s `echo` case.  `TestEchoIssue7Interface` exposed and fixed a strict-POSIX defect where `-e`, `-E`, and combined option-like operands were incorrectly consumed as Bash options.  It now covers the defined base region, the documented `-n` choice, conditional XSI escapes, output errors, and stdin preservation.  The ledger remains partial because the Profile D XSI feature-selection contract and locale-sensitive branch still need independent closure.

### `false` — verified, low (7)

Normative interface: [`false`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/false.html), no options or operands, no output, exit status greater than zero.  Bash 5.3 uses `builtins/colon.def`; Bashy dispatches `false` directly to status 1 in `builtin.go`.  `TestFalseIssue7Interface` proves silent non-zero status and that stdin and environment are unused; `TestProfileBRouteFalse` independently proves the selected Profile B/D route.  The staged Go `false` remains necessary for exec-style use, but direct shell calls select the builtin.  The canonical ledger is therefore promoted to verified.

### `fc` — evidence_gap, high (28)

Normative conditional interface: the User Portability Utilities option adds all three forms, [`fc [-r] [-e editor] [first [last]]`, `fc -l [-nr] [first [last]]`, and `fc -s [old=new] [first]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fc.html), with `-e editor`, `-l`, `-n`, `-r`, and `-s`.  There is no base `fc` synopsis outside the `[UP]` margin.  FCEDIT/HISTFILE/HISTSIZE and history-number/string selection are observable.  Bash 5.3 uses `builtins/fc.def`; Bashy uses `builtin.go` plus `history.go`, with focused editor/range/listing tests.  Under the accepted 9,337/9,337 remote pair the cloud Profile B set completed, and the targeted ARM `fc` control/candidate pair has since completed cleanly at **53/53**, so the earlier ARM cap (a history-only artifact of the superseded four-arm run) is closed — no rerun is owed.  What remains open is clause-complete interface evidence: the completed differential pairs demonstrate control/candidate parity but do not by themselves close the full FCEDIT/HISTFILE/HISTSIZE, selection, and three-form interface, so this stays an evidence gap.

### `fg` — evidence_gap, medium (12)

Normative conditional interface: [`fg [job_id]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fg.html), required only with User Portability Utilities; no options.  It foregrounds the selected/current job, continues it if stopped, waits, and returns its status.  Bash 5.3 uses `builtins/fg_bg.def`; Bashy uses the shared builtin/job carrier.  Source and non-interactive unit coverage do not close controlling-terminal and interactive selection behavior.

### `getopts` — evidence_gap, medium (10)

Normative interface: [`getopts optstring name [arg...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html), no utility options; `optstring`, `name`, and optional argument vector drive OPTIND/OPTARG and normal versus leading-colon error reporting.  Bash 5.3 uses `builtins/getopts.def`; Bashy uses `builtin.go`'s `getopts` state machine.  Profile B `TP6` found acceptance of `:` as an option character and was fixed by `87f45fee`; the remaining reset, repeated-call, explicit-arg, diagnostics, and exit matrix lacks one complete public evidence ID.

### `hash` — evidence_gap, medium (12)

Normative interface: [`hash [utility...]` and `hash -r`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html); `-r` is required, operands update/report remembered utility locations, and no operands may report the table.  Bash 5.3 uses `builtins/hash.def`; Bashy uses `builtin.go` and `cmdHashTable`, with dedicated `hash_builtin_test.go`.  Sprint 68 retired three targeted Profile B identities in repeated complete pairs, and the generated ledger correctly remains partial because its focused semantic and routing references do not close the remaining PATH-mutation, invalidation, locale, and error products.

### `jobs` — evidence_gap, low (0)

Normative conditional interface: [`jobs [-l|-p] [job_id...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/jobs.html), required only with User Portability Utilities; `-l` and `-p` are conditional required options and output is tied to the shell's current jobs.  Bash 5.3 uses `builtins/jobs.def`; Bashy uses `builtin.go` and the job formatter.  The inventory counts five UNTESTED markers against four configured TPs, yielding no likely earnable TP, but that measurement oddity does not waive the interface or prove it.

### `kill` — evidence_gap (shared-result), medium (18)

Normative base forms: [`kill -s signal_name pid...` and `kill -l [exit_status]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/kill.html), with required `-s` and `-l`.  XSI conditionally adds both `kill [-signal_name] pid...` and `kill [-signal_number] pid...`; neither shorthand is in the base synopsis.  PID operands include the POSIX process/group meanings and shell job IDs are required for the builtin.  Bash 5.3 uses `builtins/kill.def` plus jobs/signal code; Bashy uses `builtin.go`, `kill_unix.go`, and the carrier.  The accepted 9,337/9,337 remote pair records `kill:TP9` failing **identically in both A and B lanes** and `kill_NE:TP8` shared.  A defect that reproduces in both lanes attributes to the shared suite/environment, not to Bashy, so `TP9` is a shared-result gap and not a Bashy implementation gap.  The earlier Bashy-only timing regression is closed: candidate `54c05236` is patch-equivalent to the already-integrated `031d47e2`, so the job-carrier timing correction is in the shipped runtime and **no reintegration or re-measurement is owed**.  What remains is evidence, not code — attribute the shared `TP9`/`TP8` results in the suite/environment and record clause-level command evidence.  There is no reason to patch `cmds/kill`, and no shell-runtime change to re-integrate.

### `printf` — evidence_gap, critical (67)

Normative interface: [`printf format [argument...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/printf.html), no options.  Required behavior includes the specified ordinary conversions, `b` operand escapes including `\\c`, format reuse, missing/excess arguments, LC_NUMERIC, output errors, and non-zero error status.  Bash 5.3 uses `builtins/printf.def`; Bashy uses `builtin.go`'s independent formatter.  Go-applet numeric residual tests do not evidence this selected builtin, and Profile B `_NE` dispositions are not positive coverage.

### `pwd` — evidence_gap, medium (17)

Normative interface: [`pwd [-L|-P]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html), with required mutually exclusive `-L` and `-P`, logical PWD validity rules, physical fallback, absolute output, diagnostics, and status.  Bash 5.3 shares `builtins/cd.def`; Bashy uses the `pwd` case plus `validLogicalPWD`, with symlink and over-PATH_MAX tests.  Sprint 68 retired five targeted Profile B identities, but `_NE` results and Go-applet tests cannot close the builtin lane.

### `read` — evidence_gap, medium (13)

Normative interface: [`read [-r] var...`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/read.html); `-r` is required.  It reads one logical line, applies backslash continuation unless `-r`, splits by IFS, assigns the remainder to the last variable, affects the current shell, and distinguishes EOF/error status.  Bash 5.3 uses `builtins/read.def`; Bashy uses the `read` case and signal-aware input code.  Public Austin reducers cover remainder, backslash, and raw cases and signal tests cover interruption, but the complete stdin/IFS/assignment/error interface is not consolidated.

### `sh` — evidence_gap, critical (245)

Normative interfaces: [`sh` command-file, `-c` command-string, and `-s` stdin forms](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sh.html), with required `-a -b -C -e -f -h -i -m -n -u -v -x -o`, matching `+` forms, plus `-c` and `-s`; `-o/+o` take an option name.  The interface also owns shell grammar, expansion, redirection, execution, environment/startup, traps, jobs, and exit semantics.  Profile D routing is the argv0=`sh` strict route described above; Profile C is Bash 5.3 invoked as `sh`.  `sh_08:TP1` passes both arms of the accepted 9,337/9,337 remote pair and Sprint 68 retired eight selected `sh_01`/`sh_03` identities, but neither fact proves the 245-testable-TP command as a whole.  The canonical shell evidence lane is empty, so the only defensible classification is evidence gap.  The former ledger misclassification is closed by canonical commit `fe6f45d`, which records `sh` as `shell_entrypoint`.

### `test` — evidence_gap, critical (207)

Normative interfaces: [`test [expression]` and `[ [expression] ]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/test.html), no utility options.  Base requirements include the zero-through-four-argument rules, pathname/string/integer primaries, and `!`; the obsolescent `-a`/`-o` forms and parentheses are XSI-conditional.  `--` is expression data, not an option delimiter.  Bash 5.3 uses `builtins/test.def`; Bashy uses `builtin.go`/`test.go` and parser support for `[`.  The Go applet's tests are not evidence for direct shell selection, while the Profile B `_NE` disposition is only capability routing.

### `time` — evidence_gap, medium (16)

Normative interface: [`time [-p] utility [argument...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/time.html); `-p` is required, PATH selects the utility, timing output goes to standard error in the required portable format, and the result normally follows the utility status.  Bash 5.3 owns this in `parse.y`/`execute_cmd.c` (`CMD_TIME_POSIX`); Bashy owns it in `syntax.TimeClause` and runner timing code.  The parser and formatting tests prove selected cases, but no full shell evidence covers keyword ambiguity, utility-not-found/status, signal, locale, and output requirements.

### `true` — verified, low (6)

Normative interface: [`true`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/true.html), no options or operands, no output, exit status zero.  Bash 5.3 uses `builtins/colon.def`; Bashy dispatches `true` as a no-op success in `builtin.go`.  `TestTrueIssue7Interface` proves silent zero status and that stdin and environment are unused; `TestProfileBRouteTrue` independently proves the selected Profile B/D route.  The staged Go applet backs exec-style calls only.  The canonical ledger is therefore promoted to verified.

### `umask` — evidence_gap, high (27)

Normative interface: [`umask [-S] [mask]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/umask.html); `-S` is required, `mask` accepts symbolic or octal forms, no operand reports the mask, and setting it must affect the current shell and children.  Bash 5.3 uses `builtins/umask.def`; Bashy uses `builtin.go` and has dedicated symbolic/numeric/round-trip tests.  The prior `TP6/TP26` Profile B work and current tests are substantial but are not a retained command-complete evidence record.

### `unalias` — evidence_gap, low (8)

Normative interfaces: [`unalias alias-name...` and `unalias -a`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unalias.html); `-a` is required, and removal affects the current shell's subsequent parsing.  Bash 5.3 shares `builtins/alias.def`; Bashy uses `builtin.go`'s alias table.  `TestUnaliasIssue7Interface` now covers multiple operands, `-a`, `--`, missing-name continuation, missing operands, invalid options, status, and stdin preservation.  The ledger is partial until locale-sensitive diagnostics and all parse-timing consequences are independently closed.

### `wait` — evidence_gap, medium (13)

Normative interface: [`wait [pid...]`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wait.html), no options.  Operands may denote known asynchronous PIDs (and builtin job IDs); no operands waits for all known jobs, statuses must be retained, unknown IDs return 127, and signal termination produces a value greater than 128 distinguishable from normal exit status.  Bash 5.3 uses `builtins/wait.def` plus `jobs.c`; Bashy uses `builtin.go` plus the job carrier and has PID/status/reaping tests.  Those tests do not yet close all retention, multiple-operand, signal, subshell, and interactive cases.

## Required next evidence

The highest-value next batch is shell-owned, not Go-applet work: add stable
semantic `sh:<path>#<TestID>` references and independent command-specific
`bashy:<approved-path>#<TestID>` routing references for `sh`, `test`, `printf`,
`cd`, and `command`.  The trivial `true`/`false` interfaces are now verified,
and `alias`/`echo`/`unalias` have focused partial evidence.  The `kill` job-carrier correction is
already integrated (`54c05236` is patch-equivalent to the shipped `031d47e2`), so **no
reintegration or re-measurement is requested**; the residual `kill:TP9`/`TP8`
are shared A/B results to attribute in the suite/environment, not code to
change.  The targeted ARM `fc` control/candidate pair is already complete at
53/53, so **no duplicate rerun is requested** either.  The ledger's `sh`
parser-model reconciliation is already complete in `fe6f45d`.  Only after
those focused results cover stdout, stderr, status, environment and shell
effects should the canonical evidence ledger be promoted further.
