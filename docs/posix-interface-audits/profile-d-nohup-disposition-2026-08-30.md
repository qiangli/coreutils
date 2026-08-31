# Profile D `nohup:21` Status and s85 Probe Coverage — 2026-08-30

Primary contracts:
[POSIX.1 Issue 7 `nohup`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html)
and GNU Coreutils 9.11 `nohup`.

## Observed Vector

* **Exact Identity:** `nohup:21`
* **Observed Vector:**
  * New Profile D (SUT): `FAIL`
  * Paired GNU Control (2026-08-24): `PASS`
  * Paired GNU Control (2026-08-23): `FAIL`

The paired GNU control itself disagreed across dates (`2026-08-23=FAIL` vs
`2026-08-24=PASS`). That disagreement has **not** been reproduced under a
matched, controlled replay (same host, same harness build, same PTY/signal
timing), so it cannot yet be attributed to either host/harness volatility or
to a defect in this project's `nohup`. `nohup:21` stays **OPEN / UNRESOLVED**
pending that matched replay; nothing below closes it.

## s85 Probe Scope — Bounded Public Capability Evidence

The s85 probes in `cmds/nohup/nohup_s85_probe_test.go` and the existing
focused tests in `cmds/nohup` exercise `nohup`'s generic, publicly documented
behavior surface: exit-status partitioning, descriptor redirection and
`nohup.out` creation/append semantics, PATH resolution, ENOEXEC fallback, and
SIGHUP immunity. They run against this project's own harness, not against the
paired GNU control, and they do not replay the specific conditions (host,
timing, PTY allocation) that produced the `nohup:21` observation above. They
are therefore **bounded capability evidence for these generic vectors in
isolation, not a reproduction of `nohup:21` and not evidence that resolves
it**.

1. **0 / 126 / 127 Exit Status Partitioning:**
   - **0 (Success):** Executable commands complete normally and return their exact exit status (`nohup true` exits `0`).
   - **126 (Found but Not Executable):** A non-executable file found during PATH or relative lookup fails with diagnostic and exits `126` (`nohup ./blocked_cmd` exits `126`).
   - **127 (Internal Error / Not Found):** Missing utility operands (`nohup`), missing PATH executables (`nohup missing_cmd`), and redirect setup failures return `127` unconditionally.

2. **SIGHUP Disposition & Process Immunity:**
   - Invoked utilities inherit `SIGHUP` set to ignored via `/bin/sh -c "trap '' HUP; exec \"$@\""`.
   - The `nohup` process wrapper itself suppresses default termination via `signal.Notify(ch, syscall.SIGHUP)` while waiting for child execution, so the waiting process wrapper survives `SIGHUP` delivered during execution.

3. **File Descriptors & Creation / Append Semantics:**
   - Terminal standard input is redirected to write-only `/dev/null` before command lookup.
   - Terminal standard output appends to `./nohup.out` in the working directory, with fallback to `$HOME/nohup.out`.
   - A `nohup.out` created where none existed is mode `0600` (probed explicitly with an absent-file precondition).
   - Appending to an already-existing `nohup.out` preserves its pre-existing content and mode.
   - Terminal standard error follows redirected standard output, or appends to `nohup.out` when standard output is closed.

4. **PATH Search & Resolution:**
   - Command lookup via `lookCommand` evaluates `PATH` relative to `RunContext.Dir`.
   - Correctly distinguishes non-existent entries from non-executable files.

5. **Child Execution & Text Script (ENOEXEC) Fallback:**
   - Plain text scripts lacking a shebang (`#!`) are executed via `/bin/sh` fallback, preserving invocation environment and argument vectors.

## Test Coverage

Focused regression and s85 probe coverage in `cmds/nohup/nohup_s85_probe_test.go`:
- `TestS85ProbeExitCodes`: pins exit statuses `0`, `126`, and `127`.
- `TestS85ProbeDescriptorAndAppendMode`: two subtests — `AbsentFileCreatedMode0600` pins the creation mode of a newly created `nohup.out`, and `AppendPreservesExistingContentAndMode` pins append behavior and mode preservation over pre-existing content.
- `TestS85ProbeWriteAndKillHangup`: pins child process `SIGHUP` immunity and output persistence to `nohup.out`.

These tests pass on this host as of this writing. That is evidence about the
vectors listed above under the conditions this harness ran them in — it is
not a conformance percentage claim for `nohup` as a whole, and it says
nothing about why the paired GNU control disagreed across runs.

## Disposition & Matched-Replay Prerequisite

* **Disposition:** `nohup:21` remains **OPEN / UNRESOLVED**. The s85 probe
  results above narrow the generic vectors that are *not* implicated, but a
  product-owned defect specific to the `nohup:21` scenario has not been ruled
  out, and no cause (host instability, harness race, or product defect) may
  be asserted without the matched replay below.
* **Matched-Replay Prerequisite:** re-certification of Profile D for
  `nohup:21` requires a paired Subject-Under-Test/GNU-control (A/D) replay
  run under matched conditions (same host, same harness build, same
  PTY/signal timing) that reproduces or resolves the control's own
  `PASS`/`FAIL` disagreement. That replay has not been run; the prerequisite
  is unmet.
