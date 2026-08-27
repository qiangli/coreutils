# `batch` POSIX Issue 7 audit

Normative source: [POSIX.1-2016 Issue 7 `batch`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/batch.html).

## Accepted behavior and evidence

POSIX defines `batch` as equivalent to `at -q b -m now`. The implementation
takes no options or operands, reads the complete shell program from standard
input, and submits it as queue-`b` at-job with completion mail requested,
owned by the authenticated submitter, carrying the invocation environment,
working directory, and file creation mask. Queue `b` execution is
load-governed by the schedule daemon's load provider. The `job %s at %s`
confirmation goes to standard error in the invocation's `TZ` and `LC_TIME`
formats; standard output stays empty.

| Requirement | Focused evidence |
| --- | --- |
| Equivalence to `at -q b -m now` (queue, mail, load marker) | `TestBatchIsAtQueueBWithCompletionMailAndLoadMarker` |
| No options, no operands | `TestBatchUnknownFlag`, `TestBatchRejectsOperands` |
| Stdin is the persisted shell program; empty input accepted | `TestBatchPersistsJob`, `TestBatchEmptyStdin` |
| Confirmation on stderr only; write failures are errors | `TestBatchSubmissionStdoutEmpty`, `TestBatchDiagnosticOnStderr`, `TestBatchAuthenticatedRecipientAndWriteError` |
| `TZ`/`LC_TIME` govern the confirmation | `TestBatchDiagnosticUsesInvocationTZAndLCTIME` |
| Uses at's access policy (at.allow/at.deny) before scheduling | `TestBatchAccessDenyRejectsWithoutScheduling` |
| Policy fails closed on malformed lines; empty at.deny permits | `TestIssue743BatchAccessMalformedDenyFailsClosed`, `TestIssue743BatchAccessEmptyDenyPermits` |

## Issue 743 closures

- **Access policy hardening**, identical to `at`'s (see
  `cmds/at/POSIX-issue743-audit.md`): the at.allow/at.deny parser no longer
  invents comment syntax or whitespace trimming; malformed lines and stat
  failures fail closed, and a present-but-empty at.deny still permits
  everyone.

## Issue 12 re-audit (2026-08-27)

Re-verified every mandatory clause against the 2016 edition: the
`at -q b -m now` equivalence (queue `b`, completion mail, immediate
eligibility under load governance), the option-free and operand-free
synopsis, stdin as the complete shell program, retained environment/cwd/
umask, the stderr confirmation shape, exit statuses, and at.allow/at.deny
gating. No gaps found; no changes required.

## Gates

- `go test -count=20 ./cmds/batch`
- `go test -race -count=5 ./cmds/batch`
- `go vet ./cmds/batch`
- `scripts/crossvet.sh`
