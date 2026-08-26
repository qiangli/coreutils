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
with the required semantics, but is not what promotes a row. Each of the three
rows stays **partial**: the sole residual is the unimplemented `LC_MESSAGES` /
`NLSPATH` diagnostic message-catalog clause, which is shared by the entire
POSIX-required surface of this repo and is therefore not "explicitly absent".

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
for `basename` too.

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
default. This tool runs **in-process** inside the embedding shell and installs
no signal handlers of its own — signal disposition is inherited from the
embedding process, so taking the default action is one of the three permitted
`SIGALRM` outcomes and every other signal keeps its standard action by
construction. The embedder's interruption seam is `RunContext.Ctx`: a
cancelled context aborts the suspension promptly and quietly with a non-zero
status, which is the mechanism a host uses to translate a delivered signal into
an early return. Completion-by-timer (normal wakeup) and completion-by-cancel
are both pinned.

Evidence: `TestSleepIssue7IntegralDuration`, `TestSleepZeroish`,
`TestSleepErrors`, `TestSleepEndOfOptions`, `TestSleepCancel`.

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

All three are pure Go with no build-tagged or platform-conditional behavior:
the string algorithms and the timer/context suspension are byte- and
wall-clock-identical on every target. Cross-compiled `go vet` over `linux`,
`darwin`, and `windows` (plus an `aix/ppc64` build canary in the repo gate)
covers the tests as well as the sources.

## 8. Residuals (why these rows stay `partial`)

The ENVIRONMENT clause lists `LANG`/`LC_ALL`/`LC_CTYPE`/`LC_MESSAGES`/xsi
`NLSPATH`. `LC_CTYPE` has no observable effect here because splitting is
bytewise on `/` (demonstrated), but `LC_MESSAGES`/`NLSPATH` diagnostic
message-catalog localization is **not implemented** — diagnostics are fixed
English strings, consistent with the entire POSIX-required surface of this
repo. Because that clause is applicable and its residual is present (not
"explicitly absent"), none of the three rows is promoted to `verified`; each
remains `partial` with this as the exact and only residual.

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
go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=linux   go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=darwin  go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
GOOS=windows go vet ./cmds/basename ./cmds/dirname ./cmds/sleep
python3 scripts/applet-matrix.py --check
python3 scripts/posix_manifest.py --check
bash scripts/applet-test-coverage.sh
go vet  $(go list ./... | grep -v /external/)
go test $(go list ./... | grep -v /external/)
```

Results are recorded in the issue's worker log. `python3
scripts/posix_manifest.py --check` still exits non-zero solely on the
pre-existing, unrelated `sh: partial state requires focused semantic evidence`
finding (present on clean `main`); the `basename`/`dirname`/`sleep` TSV rows and
the rendered `posix-required-command-interfaces.md` were confirmed to match the
script's `render()` output exactly, so this change introduces no manifest
staleness of its own.
