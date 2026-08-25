# `logger` POSIX.1-2008 Issue 7 interface audit

Normative source: [The Open Group Issue 7, 2016 Edition `logger`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html).
Synopsis in scope: `logger string...`. Issue 7 defines **no options** for
`logger`; `-i`, `-s`, `-t`, and `-p` (plus the zero-operand stdin form) are
pre-existing historical extensions carried by this implementation and are out
of scope for expansion here. This replaces the abandoned #751 attempt; no BSD,
util-linux, or GNU extension (`-f` included) has been added, and none of the
existing extensions have been widened.

## Required interface and executable evidence

| Requirement | Implementation evidence | Test evidence |
| --- | --- | --- |
| `string...` operands concatenated in order with single spaces | `strings.Join(operands, " ")` in `run` | `TestOperandsJoinWithSingleSpaces` |
| A single empty-string operand is still an operand (message logged, stdin not read) | `len(operands) > 0` gate in `run` | `TestSingleEmptyStringOperandLogsAnEmptyMessage` |
| Empty operands still cost a separator at any position | operand join is over `operands`, never a re-split | `TestAllEmptyStringOperandsStillJoinWithSpaces`, `TestLeadingAndTrailingEmptyOperandsJoinWithSpaces` |
| `--` ends option parsing so a leading-dash operand is logged verbatim | `tool.Parse` | `TestDashDashEndsOptions` |
| Diagnostics on stderr, prefixed with the command name | `fmt.Fprintf(rc.Err, "logger: %v\n", err)` at every failure path | `TestSinkOpenFailureIsReported`, `TestSendFailureIsReported`, `TestCloseFailureIsReported`, `TestUnsupportedFlagFailsLoudly` |
| Exit status 0 on success, non-zero on any open/send/close/read/usage failure | named `status` return threading every failure path | `TestSendFailureIsNotMaskedByClose`, `TestCloseFailureIsReported`, `TestUnknownPriorityIsAnError` |
| System-log open failure is diagnosed and non-zero, nothing logged | `openSink` error path returns 1 before any `Send` | `TestSinkOpenFailureIsReported`, `TestRealSyslogSinkOpenFailureIsReported` |
| System-log send failure is diagnosed and non-zero | `emit` propagates `Send` errors | `TestSendFailureIsReported` |
| System-log close/finalization failure is diagnosed and non-zero even after a prior success | named-return `defer` in `run` | `TestCloseFailureIsReported`, `TestSendFailureIsNotMaskedByClose` |
| Zero-operand extension reads stdin, one record per line, and is clearly non-POSIX | doc comment + stdin branch in `run` | `TestNoOperandsReadsStdinOneRecordPerLine`, `TestStdinWithoutTrailingNewlineStillLogsTheLastLine`, `TestEmptyStdinLogsNothingAndSucceeds` |
| Unix system-log transport actually opens, sends, and closes over a real local socket | `dialSystemLog`/`syslogSink` in `sink_unix.go` | `TestLoggerCommandDeliversOverARealLocalSyslogSocket`, `TestRealSyslogSinkOpenSendCloseLifecycle` |
| Windows has no system log and refuses loudly rather than claiming delivery | `dialSystemLog` in `sink_windows.go` | `TestDialSystemLogRefusesOnWindows`, `TestLoggerCommandRefusesToClaimDeliveryOnWindows` (windows-only; runs natively in this repo's `windows-latest` CI leg) |
| `--help`/`--version` succeed without ever opening the transport, on every platform | `tool.Parse` short-circuit before `openSink` | `TestHelpAndVersionSucceedWithoutOpeningTheTransport` (unix, fake transport), `TestLoggerHelpAndVersionSucceedOnWindowsWithoutTheSystemLog` (windows, real transport) |

## What this audit found and changed

The prior evidence-state note (`sprint-79-consolidated.md`, `go-batch-3.md`)
flagged that `logger`'s transport was `partial`: everything exercised through
`TestSinkOpenFailureIsReported` et al. runs against `fakeSink`, a hand-rolled
stand-in — the actual production code in `sink_unix.go` (`dialSystemLog`,
`syslogSink.Send`, `syslogSink.Close`) had **zero** test coverage before this
change, on any platform. The operand-joining and empty-operand source paths
were already correct but likewise untested at the empty-string boundary.

Two changes closed this, both additive and behavior-preserving:

1. **`sink_unix.go`**: `dialSystemLog` now calls the single `dialSyslog`
   function seam, whose production value is exactly `syslog.Dial`. Production
   still passes `("", "")`, so local-socket discovery is unchanged; a test
   substitutes a function that explicitly dials a real local
   `net.ListenUnixgram` receiver it owns. This is a **real local Unix syslog
   receiver integration seam**: `cmds/logger/sink_unix_test.go` creates an
   actual `AF_UNIX SOCK_DGRAM` socket under a short-pathed `/tmp` directory
   (kept short because `sun_path` is a small fixed buffer — 104 bytes on
   Darwin/BSD, 108 on Linux — that a nested `t.TempDir()` can exceed on
   macOS), points `dialSyslog` at it, and then runs
   the **unmodified** `run()` end to end. No host syslog daemon is involved
   and none is assumed to exist; the receiver is entirely test-owned.

   This proves, against the real transport rather than a stand-in:
   - open/send/close all succeed and a real datagram is received
     (`TestLoggerCommandDeliversOverARealLocalSyslogSocket`);
   - the documented fidelity gap — `log/syslog`'s writer stamps `tag[pid]`
     into every record whether or not `-i` was given — holds under a real
     write, not just in the source comment
     (`TestRealSyslogSinkAlwaysStampsPIDRegardlessOfDashI`);
   - `syslogSink.Send`'s two code paths (same-priority `Write`, and the
     per-severity `Emerg`/`Alert`/.../`Debug` routing taken when a record's
     priority differs from the dial-time priority) both deliver correct wire
     bytes, and the wire **facility** stays the dial-time facility even when
     a record names a different one — the documented constraint that a
     syslog connection's facility cannot change after `Dial`, which is why
     `-p` is resolved before the sink opens
     (`TestRealSyslogSinkOpenSendCloseLifecycle`);
   - a real, deterministic open failure — nothing listening at the dial
     path — is diagnosed and non-zero
     (`TestRealSyslogSinkOpenFailureIsReported`).

2. **`sink_windows_test.go`** (new, `//go:build windows`): a Windows
   disposition test that calls `dialSystemLog` directly and also runs `run()`
   unfaked, proving the refusal is loud (names "Windows", exits non-zero) and
   that `--help`/`--version` still succeed without ever reaching the
   transport. This repo's CI runs `go test` natively on `windows-latest`
   (`.github/workflows/test.yml`), so this test executes for real in CI, not
   only type-checked by cross-vet.

3. **`logger_test.go`**: three new tests close the empty-string-operand gap
   named in the audit brief — a lone empty operand (`logger ''`) must log an
   empty message and must not fall through to the zero-operand stdin
   extension; multiple empty operands still join with one space each; and a
   leading/trailing empty operand costs a separator at that end too. No
   source change was needed for these — `strings.Join` and the
   `len(operands) > 0` gate were already correct — only the evidence was
   missing.

No option was added, widened, or removed. `-f`, `-u`, and every other
BSD/util-linux/GNU `logger` extension remain unimplemented and fail loudly
through the existing `tool.Parse` strict-flag contract
(`TestUnsupportedFlagFailsLoudly`); this audit did not touch that contract.

## Honest residuals

- **Host-daemon integration.** The real-receiver tests prove the actual
  `log/syslog`-based transport code path (dial/send/close, wire format,
  severity routing, facility pinning) works against a real Unix domain
  socket. They do not and cannot prove end-to-end delivery through an actual
  running `syslogd`/`rsyslogd`/`syslog-ng` on a certification host — that
  requires a live daemon and is host state, not something a hermetic unit
  test can own. `log/syslog.Dial("", "", ...)` (the unchanged production
  call) tries Unix datagram and then Unix stream connections at `/dev/log`,
  `/var/run/syslog`, and `/var/run/log`; it does not fall back to UDP/TCP.
  The explicit-datagram test therefore does not prove production empty-network
  discovery, the Unix-stream fallback, daemon policy, or durable persistence.
  Those remain certification-host evidence rather than package behavior a
  hermetic test can guarantee.
- **`-i` cannot be honoured on the syslog side.** `log/syslog`'s writer
  stamps `tag[pid]` into every record unconditionally; there is no API to
  suppress the pid. This was already documented in `sink_unix.go` and is now
  additionally backed by `TestRealSyslogSinkAlwaysStampsPIDRegardlessOfDashI`
  against a real socket. `-i` remains observable only on the `-s` stderr
  copy, which this package formats itself.
- **Diagnostic language.** Diagnostics are English-only. Issue 7 says message
  language and cultural conventions *should* follow `LC_MESSAGES`; it does not
  require every utility to ship translated catalogs. This is a product
  localization limitation, not by itself a failed utility interface.
- **Windows** has no system log at all; the refusal is the documented and
  tested disposition, not a residual gap.

## Acceptance gates run for this change

- `go build ./cmds/logger/...`
- `go vet ./cmds/logger/...`
- `go test ./cmds/logger/...` (verbose, all green)
- `go test -count=20 ./cmds/logger/...`
- `go test -race -count=5 ./cmds/logger/...`
- `GOOS=linux go vet ./cmds/logger/...`
- `GOOS=darwin go vet ./cmds/logger/...`
- `GOOS=windows go vet ./cmds/logger/...`
- `GOOS=freebsd go vet ./cmds/logger/...`
- `GOOS=aix GOARCH=ppc64 go build ./cmds/logger/...`
- `go build -o /tmp/coreutils-check ./cmd/coreutils` (multicall binary still links)
- `go vet $(go list ./... | grep -v /external/)` (repository-wide scope, unaffected)
