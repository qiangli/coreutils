# `crontab` POSIX Issue 7 audit

Normative source: [POSIX.1-2016 Issue 7 `crontab`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/crontab.html).

## Accepted behavior and evidence

Both synopsis forms are implemented: `crontab [file]` (stdin when no operand)
replaces the invoking user's crontab, and `-l`/`-e`/`-r` list, edit (via
`EDITOR`, default `vi`), and remove it; the three options are mutually
exclusive and take no operand. Installation is atomic: the whole source is
parsed first, every schedule validated, and only a fully valid table replaces
the stored one — source bytes are preserved verbatim for `-l` round-trips.
Entries are five time-and-date fields plus a command executed by `sh -c`
under a default environment defining `HOME`, `LOGNAME`, `PATH`, and `SHELL`,
with output mailed to the user. An unescaped `%` ends the command; the
remainder becomes the job's standard input with later `%`s as newlines.
cron.allow/cron.deny gate every operation with allow-precedence and a
privileged-only default.

| Requirement | Focused evidence |
| --- | --- |
| Install/list/remove/edit round trip; byte-for-byte source | `TestCrontabInstallListRemove`, `TestCrontabRoundTrip`, `TestCrontabPreservesWholeSourceAndEditorInputByteForByte` |
| Mutually exclusive modes, operand limits, `-u` fails closed | `TestCrontabRejectsConflictingModesAndExtraOperands`, `TestCrontabUserOptionFailsClosed` |
| Atomic rejection of invalid tables and schedules | `TestCrontabBadLine`, `TestCrontabInvalidScheduleIsAtomic`, `TestCrontabReinstallReplaces` |
| Comments/blank lines skipped; command whitespace preserved | `TestCrontabParseSkipsComments`, `TestCrontabPreservesCommandInternalWhitespace` |
| `%` command/stdin split and `\%` escape | `TestPercentCompilationAndExplicitShell` |
| Backslash escapes only `%`; other backslashes are literal command data | `TestIssue743BackslashOnlyEscapesPercent`, `TestIssue743TrailingBackslashIsLiteral` |
| Default execution environment, absolute shell, default PATH | `TestCrontabPersistsShellProgramAndContext`, `TestCronExecutionUsesAbsoluteShellAndDefaultPATH` |
| Access policy one-username-per-line, fail-closed | `TestAccessPolicyRequiresExactlyOneUsernamePerLine`, `TestMalformedAccessPolicyFailsClosed` |
| Stdin replace; `-` operand is a literal pathname | `TestCrontabStdinReplace`, `TestCrontabDashOperandIsLiteralPathname` |
| Store isolation from at jobs | `TestRunContextSelectsIsolatedStoresAndPreservesAtJobs` |

## Issue 743 closure

- **Backslash handling in the command field.** The parser previously treated
  a backslash as a generic escape, dropping it before *any* following byte,
  so a canonical crontab command such as `printf 'a\tb\n'` was installed as
  `printf 'atbn'`. Per the historical crontab implementations that POSIX
  codifies, a backslash escapes only a following percent-sign (`\%` → `%`,
  suppressing the newline translation); before any other byte — and at end
  of line — it is command data the shell must receive unchanged. The
  left-to-right scan also makes `\\%` yield a literal `\%`.

## Residuals

- Vixie-style environment-assignment lines (`NAME=value`) and the schedule
  syntax accepted by the cron engine beyond the POSIX list/range/asterisk
  forms (names, steps) are compatible upstream-crontab extensions; a line
  that is valid POSIX cannot collide with them. `SHELL=` values are
  validated at install time.
- POSIX does not specify behavior for `-l`/`-r` when no crontab exists; the
  empty table lists as empty output with exit 0 (an established Bashy
  decision, `TestCrontabListEmpty`).
- Editing uses `EDITOR` as a single command name (the historical `execlp`
  behavior); shell-interpreted editor strings are not a POSIX requirement.

## Gates

- `go test -count=20 ./cmds/crontab`
- `go test -race -count=5 ./cmds/crontab`
- `go vet ./cmds/crontab`
- `scripts/crossvet.sh`
