# POSIX `renice` interface closure (S79, issue 750)

This audit uses The Open Group POSIX.1-2008, Issue 7, 2016 Edition `renice`
utility page (XCU) as the sole normative reference
(<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html>).
GNU/util-linux parity is out of scope. It supersedes the conservative
source-token audit recorded for `renice` in the generated interface ledger.
The consolidated Sprint 79 ledger, generated manifests, and shared command
matrices are intentionally unchanged here (see "Manifest reconciliation
owed" below).

## Interface decisions closed by this pass

- **The obsolescent first-operand form is refused, not reinterpreted.**
  Issue 7's only synopsis is `renice [-g|-p|-u] -n increment ID...`; the
  RATIONALE records that the `renice nice_value ...` forms were removed
  from the standard (Issue 6), and historically they took an *absolute*
  nice value where `-n` takes an *increment*. The previous implementation
  accepted the first operand as an increment — a silent wrong answer under
  both readings. `renice 5 12` is now an exit-2 usage error whose message
  names `-n` and the refused obsolescent form.
- **Guideline 9 exemption implemented as positional selector switching.**
  The OPTIONS section says renice conforms to the Utility Syntax
  Guidelines "except for Guideline 9", and each of `-g`/`-p`/`-u` reads
  "interpret the *following* operands ...". Selectors may therefore appear
  between operands and re-interpret the operands after them, so one
  invocation mixes process, group, and user IDs. The former pflag model
  (three mutually exclusive booleans, position-blind) could not express
  this; parsing is now hand-rolled and order-preserving. `-p` is the
  default. `-n` remains one per-invocation value, but the Guideline 9
  exception also permits it after an operand; because execution starts only
  after the complete argv is parsed, the one value applies unambiguously to
  every ID. A duplicate `-n` remains a usage error. The hand parser also
  retains the previously supported `--increment`/`--pgrp`/`--pid`/`--user`
  and `-h`/`-V` extensions, including unambiguous long-option prefixes.
- **STDOUT is "Not used."** The former `%d: old priority %d, new priority
  %d` success line was a conformance violation and is removed; success is
  silent.
- **Bounds are the kernel's, not a userspace guess.** The DESCRIPTION
  makes the nice-value bounds implementation-defined with clamping ("the
  limit whose value was exceeded shall be used"), and POSIX
  `setpriority()` itself mandates that clamping. The former hardcoded
  `[-20, 19]` clamp was provably wrong on this host (Darwin's limit is
  `[-20, 20]`). `{NZERO}` has a minimum of 20, not a fixed value, so a
  replacement ±40 guess would be wrong too. The computed
  `current + increment` is now passed through to `setpriority()`, saturated
  only at the signed C-int boundary required by that API; the kernel applies
  its actual nice bounds. Decimal magnitudes outside the implementation's
  `int64` arithmetic range fail clearly as usage errors rather than being
  silently approximated.
- **Linux `getpriority` raw-encoding defect fixed.** `x/sys`'s
  `unix.Getpriority` on Linux is a raw syscall wrapper: the kernel returns
  `20 - nice` (the raw ABI reserves negative returns for errno) and no
  libc conversion happens. The old code consumed that value as a nice
  value, so on Linux every read was mirrored around 20 (nice 0 read as
  20) and `renice -n 0 -p $$` would silently set nice 19. This was
  invisible to a darwin-only test run. `prio_linux.go` now undoes the
  encoding; Darwin/BSD/Solaris/AIX report errors out of band and return
  the true value (`prio_unix_libc.go`).
- **Increment direction.** The 2016-edition `-n` text reads "Positive
  increment values shall cause a lower nice value", which contradicts the
  XBD Nice Value definition, the `-n` clause's own privilege sentence
  (negative increments "may require appropriate privileges"), and
  `nice()`/`setpriority()`. This is the residue of the terminology
  reversion the RATIONALE describes (an editorial sweep of "system
  scheduling priority" → "nice value" without flipping lower/higher). The
  implemented arithmetic is the only self-consistent one, matching every
  historical implementation: the increment is *added* to the current nice
  value, so positive increments produce a numerically higher nice value
  (less favorable scheduling, no privilege needed) and negative increments
  a lower one (privileged).
- **Collective selectors preserve per-process increments on Linux.**
  `getpriority(PRIO_PGRP/PRIO_USER)` returns one collective minimum and a
  collective `setpriority()` overwrites heterogeneous members with one
  value. Linux therefore enumerates `/proc`, selects process-group members
  from `/proc/PID/stat` and saved-set-UID matches from `/proc/PID/status`,
  then reads and adjusts each PID independently. Metadata errors fail the
  selector closed; per-process scheduler failures are diagnosed while the
  remaining snapshot members continue. Unix platforms without a robust
  membership enumerator fail `-g`/`-u` once with an explicit unsupported
  diagnostic rather than approximating.
- **ID 0 is embedding-safe.** The standalone multicall marks its
  `RunContext` as a dedicated command process, where `-p 0` and `-g 0` can
  retain the kernel's calling-process/group meaning. In-process embeddings
  fail those operands before a scheduler call, so they cannot accidentally
  renice the long-lived host process or group. Linux `-u` is enumerated by
  the resolved saved-set UID and does not depend on collective ID-0 syscall
  semantics.
- **Fail-closed non-Unix disposition.** On `!unix` targets (Windows,
  js/wasip1) `renice` fails closed once per invocation — after parsing, so
  `--help`/`--version`/usage diagnostics keep their cross-platform
  behavior, and before any ID is touched — with a diagnostic naming the
  platform, instead of erroring one ID at a time or mapping Windows
  priority classes onto nice values. The package now builds on every
  crossvet target (windows, linux, darwin, aix, js/wasip1-wasm).

## Clause map and evidence

All evidence is in `cmds/renice/`. Hermetic tests drive the grammar and
dispatch through an invocation-owned scheduler seam (`scheduler`
interface, threaded through `runWith`, never package-global state); a
recording fake emulates POSIX `setpriority()` clamping. Real-runtime tests
drive `run()` against the live kernel under strict privilege containment.

- **SYNOPSIS / required `-n`:** `TestMissingIncrementOptionIsUsageError`,
  `TestObsolescentFirstOperandFormIsRefusedLoudly` (also proves no
  scheduler call happens on a usage error),
  `TestMissingIncrementArgumentAndMissingID`,
  `TestDuplicateIncrementIsRefusedAndLateIncrementIsAccepted`.
- **OPTIONS grammar (Guidelines 3–10, minus 9):** separate and attached
  option-arguments (`TestIncrementForms`), clustered shorts
  (`TestClusteredOptionsParse`: `-gn1`, `-pg`), `--` termination and lone
  `-` as an operand (`TestDoubleDashEndsOptionsAndLoneDashIsOperand`),
  unknown options and long-option value-rejection exit 2
  (`TestUnknownOptionsAreUsageErrors`). Retained long selectors,
  `--increment`, and `-h`/`-V` are covered by
  `TestRetainedLongOptionsAndHelpVersionAliases`; universal
  `--help`/`--version` and prefix expansion by `TestHelpAndVersion`.
- **`-n increment` parsing and semantics:** signed decimal only
  (`TestInvalidIncrementIsUsageError`); relative to the *current* value
  with scheduler-side clamping
  (`TestIncrementIsRelativeAndBoundsAreSchedulerClamped`); C-int-only
  saturation arithmetic (`TestRequestedNiceSaturates`); live proof that the current
  value is read, not assumed
  (`TestRealZeroIncrementPreservesOwnNiceAndIsSilent`, which on Linux also
  pins the getpriority encoding fix).
- **`-g`/`-p`/`-u` selection:** exact selector interpretation per operand
  position, including the default and mid-line switching
  (`TestPositionalSelectorSwitchingDispatchesExactWhich`), synopsis order
  (`TestSelectorBeforeIncrementMatchesSynopsis`), heterogeneous per-member
  arithmetic and continuation
  (`TestCollectiveIncrementIsPerProcessAndContinuesWithinSelector`), Linux
  `/proc` parsing/membership tests (`prio_linux_test.go`), and live
  PRIO_PROCESS plus Linux PRIO_PGRP adjustment of an owned parked child
  (`TestRealPositiveIncrementOnOwnedChildAndGroup`).
- **OPERANDS / ID parsing:** `-g`/`-p` operands are unsigned decimal
  integers bounded to signed `pid_t` range; signs, fractions, and
  4294967296 are refused (`TestInvalidIDsFailPerOperand`). `-u` resolution
  is name-first, then an unsigned 32-bit UID where the host Go `int` can
  represent it, else a clear per-operand failure
  (`TestUserOperandResolution`, `TestUserNameOperandResolvesToUID`,
  `TestFullWidthNumericUIDWhereHostIntPermits`). Dedicated versus embedded
  ID-0 behavior is pinned by
  `TestZeroProcessIDFailsClosedInEmbeddingAndPassesInDedicatedProcess`.
- **Ordered mixed success/failure:** operands are attempted in
  command-line order; each failure gets one stderr diagnostic naming the
  operand; processing continues; exit status is 1
  (`TestOrderedMixedSuccessAndFailureContinues`).
- **STDIN / INPUT FILES:** standard input is never read and there are no
  utility input-file operands. Linux collective selection reads the kernel's
  `/proc` process metadata as its scheduling interface; it does not consume
  user input or a pathname operand.
- **STDOUT ("Not used.") and STDERR (diagnostics only):**
  `TestSuccessIsSilentOnStdout`, plus the stdout assertions in every
  real-runtime test including the failure paths.
- **EXIT STATUS / CONSEQUENCES OF ERRORS:** 0 on success; 1 when any ID
  fails (diagnosed, processing continued); 2 for usage errors — the
  repo-wide documented deviation inside POSIX's ">0 an error occurred".
  Permission and kernel errors surface per operand:
  `TestRealNegativeIncrementWithoutPrivilegeFailsPerOperand` (EPERM/EACCES
  path, skipped honestly when the environment is privileged, with the
  change undone if it unexpectedly succeeded) and
  `TestRealNonexistentProcessDiagnostic` (ESRCH path).
- **Privilege rule ("shall not alter ... without appropriate
  privileges"):** enforced by the kernel through `setpriority()` and
  propagated as the per-operand error status; see the negative-increment
  real test above.
- **Platform disposition:** `TestNonUnixFailsClosedAfterParsing`
  (`prio_other_test.go`) covers hosts without POSIX nice values. The live
  Unix test covers the explicit non-Linux collective-selector failure;
  Linux-specific parser/enumerator tests compile only where `/proc` is the
  implementation.
- **ENVIRONMENT VARIABLES:** renice consults no environment variable
  beyond the locale set; diagnostics are deterministic English
  (`LC_ALL=C` agent contract), see residuals.

## Manifest reconciliation owed (conductor)

Shared generated manifests were deliberately left untouched, matching the
established worker/conductor split (`docs: reconcile ... POSIX evidence`
commits). The `renice` rows are now stale in exactly these ways:

- `docs/posix-required-command-interfaces.tsv`/`.md`: `parser_model` is
  `manual` (the package no longer calls `tool.NewFlags`); the obsolescent
  special-token sentence no longer applies (the form is refused); Go
  evidence lane should cite, e.g.:
  `cmds/renice/renice_test.go#TestMissingIncrementOptionIsUsageError;`
  `cmds/renice/renice_test.go#TestPositionalSelectorSwitchingDispatchesExactWhich;`
  `cmds/renice/renice_test.go#TestOrderedMixedSuccessAndFailureContinues;`
  `cmds/renice/renice_test.go#TestIncrementIsRelativeAndBoundsAreSchedulerClamped;`
  `cmds/renice/renice_test.go#TestSuccessIsSilentOnStdout` (the five
  previously cited test IDs were superseded and no longer exist).
- `docs/applet-matrix.md`/`.tsv`: the renice file counts changed
  (regenerate with `scripts/applet-matrix.py`).

Baseline note recorded for honesty: `posix_manifest.py --check` and
`posix_manifest_test.py` were already red on this workspace's base commit
(first failure: `sh` row), and `cmds/batch` + `cmds/getconf` each have one
pre-existing host-environment test failure; none of these involve renice.

## Residuals (honest limits)

- **Linux collective snapshots are inherently non-atomic.** `/proc` is
  fully scanned before any member is changed, and malformed or unreadable
  metadata fails closed. Processes can still enter or leave a group, change
  credentials, exit, or change their own nice value between the snapshot
  and its per-PID syscall. A disappearing PID is diagnosed if it survived
  the membership snapshot; a process that vanished while the snapshot was
  being read is skipped because it no longer exists. There is no Linux API
  that atomically combines saved-set-UID/process-group selection with a
  distinct relative update per member.
- **Collective selectors are explicitly unsupported off Linux.** Darwin,
  the BSDs, Solaris, and AIX keep exact `-p` behavior but return a clear
  status-1 diagnostic for `-g`/`-u`; they no longer silently use collective
  minimum-then-set or effective-UID semantics. This is supported-vs-clear-
  unsupported, not a POSIX-complete implementation on those hosts.
- **Full-width UID on 32-bit Go hosts.** The `x/sys/unix` priority wrappers
  take Go `int`. Numeric UIDs through `UINT32_MAX` are retained on 64-bit
  hosts; values above `MaxInt32` fail clearly on 32-bit hosts rather than
  wrapping to a different user.
- **No real-runtime `-u` test, by design.** A real saved-UID selector can
  touch every matching process on the host. The `/proc` saved-UID field and
  heterogeneous per-member behavior are covered through filesystem and
  scheduler seams instead of risking unrelated processes.
- **Locale:** diagnostics are deterministic English; no translated
  `LC_MESSAGES`/`NLSPATH` catalogs ship, the same Issue 7 locale
  limitation recorded for `nice` (issue 730).
- **Adjacent defect, out of this issue's scope:** `cmds/nice`'s
  `priority_unix.go` `currentPriority()` consumes `unix.Getpriority`
  unconverted and therefore has the same Linux raw-encoding defect fixed
  here for renice (on Linux it reads ~20 and clamps children to nice 19).
  Left untouched to keep this workspace single-command; flagged for a
  follow-up issue.
