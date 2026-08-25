# `at` POSIX Issue 7 audit

Normative source: [POSIX.1-2016 Issue 7 `at`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/at.html).

## Accepted behavior and evidence

All five synopsis forms are implemented: `-t time_arg` submission, `timespec`
submission, `-r at_job_id...` removal, `-l -q queuename`, and `-l
[at_job_id...]`, with `-m`, `-f file`, and `-q queuename` on the submission
forms only. Cross-family combinations are exit-2 usage errors. The job is the
complete standard-input (or `-f file`) shell program handed to `$SHELL -c`
(default `sh`); the invocation environment, working directory, and file
creation mask are persisted with the job, which runs in its own session with
no controlling terminal, mails output to the submitting user, and honors `-m`
completion mail. Queue names are single lowercase letters, `a` default,
`b` load-governed. `TZ` selects the submission zone (a trailing `utc`
timespec token overrides it) and `LC_TIME` governs month/weekday name parsing
and the `job %s at %s` stderr confirmation plus the `-l` output times.

| Requirement | Focused evidence |
| --- | --- |
| Timespec grammar (clock, noon/midnight, am/pm, adjacent forms, date, weekday, today/tomorrow, increments, now) | `TestParseAtTimespec`, `TestParseLicensedAtGrammar`, `TestAtHHMM`, `TestAtNoon`, `TestAtMidnight` |
| `-t` touch grammar incl. leap-second carry | `TestParseAtTouchTime`, `TestParseAtTouchTimeLeapSecond`, `TestAtTouchTimeAndInvalidQueue` |
| Synopsis families and invalid combinations fail with usage errors | `TestAtRejectsCrossFamilySynopsisCombinations`, `TestAtUnknownFlag` |
| Persisted shell program, cwd, environment, umask | `TestAtJobRetainsShellProgramAndWorkingDirectory`, `TestAtAcceptsEmptyAndBlankStdin` |
| Queue submission, `-l -q` filtering, queue `b` load governance | `TestAtQueueSubmissionAndListFiltering`, `TestAtQueueBIsLoadGovernedAndUsesAuthenticatedRecipient` |
| Owner isolation for list/remove; atomic multi-id removal | `TestAtOwnerIsolation`, `TestAtRemoveMixedIDsIsAtomic`, `TestAtRemoveNonexistent` |
| `LC_TIME`/`TZ` accepted and written formats | `TestAtLCTimeParsingAndUnsupportedLocale`, `TestAtListUsesInvocationTZAndLCTIME` |
| Daemon execution keeps submission context, session isolation | `TestAtSubmissionContextSurvivesDaemonExecution`, `pkg/schedule` process-group tests |
| `-l` with an unknown at_job_id is a diagnosed error, known jobs still listed | `TestIssue743ListUnknownJobIDFails` |
| Past `-t` time is rejected naming the `-t` argument | `TestIssue743TouchTimePastDiagnosticNamesArgument`, `TestAtPastTime` |
| at.allow precedence, at.deny, default privileged-only | `TestAtAccessAllowTakesPrecedenceAndDenyRejects` |
| Access policy is one-username-per-line and fails closed on malformed lines and stat errors; empty at.deny permits everyone | `TestIssue743AtAccessMalformedPolicyFailsClosed`, `TestIssue743AtAccessStatErrorFailsClosed`, `TestIssue743AtAccessEmptyDenyPermits` |

## Issue 743 closures

- **`-l` unknown at_job_id.** POSIX exit status 0 means the utility
  "successfully submitted, removed, or listed a job or jobs". A requested
  at_job_id naming no job of the caller's is now diagnosed (`no job "id"`)
  with exit 1, symmetric with `-r`; foreign-owner and non-at jobs remain
  indistinguishable from missing jobs.
- **Past `-t` diagnostic.** The rejection previously interpolated the joined
  timespec operands, which are empty in the `-t` form, producing
  `time "" is in the past`. It now names the actual requested argument.
- **Access policy hardening.** at.allow/at.deny parsing previously invented a
  `#` comment syntax, trimmed interior whitespace, and skipped a directory on
  non-ENOENT stat errors (falling through toward a later allow file or the
  privileged default). POSIX defines the format as one user name per line;
  the parser now matches crontab's already-hardened cron.allow behavior:
  malformed lines and stat failures fail closed, while a present-but-empty
  at.deny still permits everyone.

## Residuals

- `-q` accepts queues `a`–`z`; queues other than `a` and `b` have
  implementation-defined semantics and are stored verbatim (only `b` is
  load-governed). Uppercase queues are not an at feature in POSIX and are
  rejected.
- Timezone name suffixes in a timespec are implementation-defined; only
  `utc` is recognized, matching the historical implementations.
- `at.allow`/`at.deny` locations are searched in the XSI and Linux
  conventional directories (`/usr/lib/cron`, `/etc/cron.d`, `/etc`); the
  first policy file found governs.
- Mail delivery is provided by the schedule daemon's mail provider; jobs
  requiring mail fail closed when no provider is configured
  (`pkg/schedule` evidence).

## Gates

- `go test -count=20 ./cmds/at ./cmds/atq ./cmds/atrm`
- `go test -race -count=5 ./cmds/at ./cmds/atq ./cmds/atrm`
- `go vet ./cmds/at ./cmds/atq ./cmds/atrm`
- `scripts/crossvet.sh`
