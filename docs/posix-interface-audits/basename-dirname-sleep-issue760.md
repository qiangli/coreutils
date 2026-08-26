# `basename` / `dirname` / `sleep` POSIX.1-2016 Issue 7 Audit

Scope: the three string/timing utilities `basename string [suffix]`,
`dirname string`, and `sleep time` against The Open Group POSIX.1-2008 Issue 7,
2016 Edition utility pages, audited end-to-end against `cmds/basename`,
`cmds/dirname`, and `cmds/sleep` as of main `6ffca6b`.

* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html>
* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html>
* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html>

GNU-only behavior (`basename`/`dirname` `-a`/`-s`/`-z` and multiple operands,
`sleep` fractional values, `s`/`m`/`h`/`d` suffixes, and multi-operand summing)
is outside the Issue 7 evidence surface. It is present and does not interfere
with the required semantics, but is not what promotes a row. Each row remains
**partial** under the consolidated Sprint 79 fail-closed policy while shared
process-boundary and advertised-platform runtime evidence remains incomplete.
The absence of shipped translated catalogs is recorded as a localization
product gap, not treated as a command-interface blocker by itself.

## 1. `basename` — Options, Operands, and the `--` Special Token

POSIX `basename` defines **no options**; the string-reduction algorithm is
implemented in `base()` exactly as the OPERANDS section specifies: strip
trailing `/`; reduce an all-slash string to `/`; remove the prefix through the
last remaining `/`; then remove `suffix` only when it is a *proper* (non-whole)
suffix. The two permitted implementation-defined choices are taken and pinned:
a null `string` yields the empty string, and `//` reduces to `/`.

The `--` Utility Syntax Guideline 10 token is the only documented special
token: because `basename` has no options, `--` is the sole way to supply a
`string` operand that begins with `-`. This was previously unexercised; it is
now demonstrated to route an option-like first operand (`-a`, `--zero`, `-s`)
to the string reducer rather than the flag parser, including the classic
two-operand `-- string suffix` form.

Evidence: `TestBasename`, `TestBasenameEndOfOptions`, `TestBasenameErrors`.

## 2. `basename` — Byte-Safe, Locale-Independent Splitting

The component split is bytewise on `/` (0x2F), which can never be a byte of a
multi-byte character in a POSIX-conformant encoding, so the result is
independent of `LC_CTYPE` and non-ASCII component bytes are preserved verbatim.
This matches the parity clause already pinned for `dirname` and is now pinned
for `basename` too, including a non-UTF-8 byte fixture that would expose a
rune-decoding or normalization implementation.

Evidence: `TestBasenameByteSafety`.

## 3. `dirname` — Operand Reduction and `--`

`dirOf()` implements the OPERANDS algorithm without path canonicalization
(`a/./b` → `a/.`, matching Issue 7 and GNU, which is why `filepath.Dir` is not
used): strip trailing `/`; if nothing remains yield `/`; else remove the last
`/`-separated component and any residual trailing `/`, yielding `/` for an
all-slash prefix and `.` when the string has no `/`. The `--` token is
exercised so a `string` beginning with `-` is treated as an operand, and byte
safety on `/` is pinned for non-ASCII components exactly as for `basename`.

Evidence: `TestDirname`, `TestDirnamePOSIXSingleOperandByteSafety`.

## 4. `sleep` — Time Operand and Asynchronous Events

The Issue 7 `time` operand is a non-negative decimal integer; `sleep` suspends
for at least that many integral seconds. Integral suspension, the zero case,
and rejection of a non-numeric or negative operand are pinned. The `--` token
ends option parsing (Issue 7 defines no command-specific special token): `--`
before a valid `time` still suspends, and `--` before an option-like operand
routes it to time-operand parsing, where a negative value is an invalid
interval, never a flag.

**Asynchronous events.** Issue 7 permits `sleep`, on receiving `SIGALRM`, to
terminate normally with exit status `0`, to effectively ignore it, or to take
the signal's default action; the action for all other signals is the standard
action. Unix process tests now invoke `cmd.Run` inside an actual child process,
wait for an explicit readiness handshake, then deliver the signal. The
SIGALRM test accepts only the three permitted dispositions and bounds the
effective-ignore path by using a one-second POSIX operand under a five-second
watchdog. The separate SIGTERM test requires a signal-terminated wait status
whose signal is exactly SIGTERM. The helper is selected with the test binary's
real `-test.run` flag; it does not pass `sleep` as an ordinary test-binary
argument and accidentally rely on nonexistent multicall dispatch.

The embedder's separate interruption seam remains `RunContext.Ctx`: a
cancelled context aborts promptly and quietly with non-zero status. That test
is useful host integration coverage, but is not substituted for process-level
signal evidence.

Evidence: `TestSleepIssue7IntegralDuration`, `TestSleepZeroish`,
`TestSleepErrors`, `TestSleepEndOfOptions`, `TestSleepCancel`,
`TestSleepSIGALRMPermittedDisposition`, `TestSleepSIGTERMStandardAction`.

## 5. Standard Input, Output, and Error

All three utilities declare STDIN "Not used." This is now pinned positively for
each with a standard-input reader that fails the test if it is ever read,
across both a successful run and an error run. Standard output carries only the
result string plus a terminator (`basename`/`dirname`); `sleep` writes nothing
to standard output. Standard error carries only diagnostics (missing operand,
invalid interval, unknown flag, and — for `basename`/`dirname` — a
standard-output write failure). Write-failure handling and its exit status are
pinned for `basename` and `dirname`.

Evidence: `TestBasenameDoesNotConsumeStdin`, `TestDirnameDoesNotConsumeStdin`,
`TestSleepDoesNotConsumeStdin`, `TestBasenameWriteErrors`,
`TestDirnameWriteErrors`.

## 6. Exit Status

`0` on success; a usage error (missing operand, unknown flag, extra operand for
`basename`, invalid `sleep` interval) exits `2` per the repo-wide documented
deviation of using `2` for usage errors where GNU sometimes uses `1`; a
standard-output write failure exits `1`. All three are pinned.

Evidence: `TestBasenameErrors`, `TestDirnameErrors`, `TestSleepErrors`,
`TestBasenameWriteErrors`, `TestDirnameWriteErrors`.

## 7. Platform Disposition

The three implementations are pure Go; the string algorithms and timer/context
suspension contain no platform branches. Process signals are necessarily
platform-specific: focused runtime evidence is Unix-tagged, while Windows has
no POSIX signal contract and other advertised POSIX targets receive compile/vet
coverage rather than a runtime signal fixture in this workspace. Cross-vet
covers `linux`, `darwin`, `windows`, and `aix/ppc64` sources and tests.

## 8. Residuals (why these rows stay `partial`)

The ENVIRONMENT clause lists `LANG`/`LC_ALL`/`LC_CTYPE`/`LC_MESSAGES`/xsi
`NLSPATH`. `LC_CTYPE` has no observable effect on the pathname split because
`/` is located bytewise (demonstrated). Diagnostics are fixed English and no
translated catalogs ship, but Sprint 79's consolidated policy explicitly says
that absence is a localization product gap rather than, by itself, a failed
utility interface. The rows remain conservatively `partial` because this wave
does not supply runtime process-boundary/platform evidence across every
advertised POSIX target. For `sleep`, Unix process-level SIGALRM and SIGTERM are
now covered; non-Unix POSIX runtime breadth and embedding-host signal
translation remain outside this focused evidence.

GNU extensions (`basename`/`dirname` `-a`/`-s`/`-z` and multiple operands;
`sleep` fractional values, `s`/`m`/`h`/`d` suffixes, and multi-operand summing)
are supported outside the required Issue 7 interface and are not part of the
promotion basis; they are retained in regression coverage only to prove they do
not perturb the required semantics.

## 9. Gate Record

Required local gate for this issue, run on 2026-08-25:

```sh
go test -count=20 ./cmds/basename ./cmds/dirname ./cmds/sleep
POSIXLY_CORRECT=1 go test -count=20 ./cmds/basename ./cmds/dirname ./cmds/sleep
go test -race -count=5 ./cmds/basename ./cmds/dirname ./cmds/sleep
POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/basename ./cmds/dirname ./cmds/sleep
go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=linux GOARCH=386   go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=linux GOARCH=amd64 go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=darwin GOARCH=arm64 go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=windows GOARCH=amd64 go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=aix GOARCH=ppc64 go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
./scripts/fmtcheck.sh
python3 scripts/applet-matrix.py --check
python3 scripts/posix_manifest.py --check
bash scripts/applet-test-coverage.sh
```

All focused default, POSIX, race, vet/cross-target-vet, formatting,
applet-coverage, and matrix commands above passed. The sole exception,
`python3 scripts/posix_manifest.py --check`, exits non-zero on the pre-existing,
unrelated `sh: partial state requires focused semantic evidence` finding;
independently calling the
manifest's `read_manifest()`, `render()`, and `validate_rendered()` confirmed
that the TSV and rendered Markdown are byte-for-byte synchronized.

The Unix signal products are runtime tests on the review host; cross-vet only
type-checks them for the other Unix targets and is not described as runtime
signal evidence there.
