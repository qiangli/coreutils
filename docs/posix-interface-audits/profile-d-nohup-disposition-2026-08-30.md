# Profile D `nohup` Control-Instability Disposition — 2026-08-30

Primary contracts:
[POSIX.1 Issue 7 `nohup`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html)
and GNU Coreutils 9.11 `nohup`.

## Observed Vector Analysis

* **Exact Identity:** `nohup:21`
* **Observed Vector:**
  * New Profile D (SUT): `FAIL`
  * Paired GNU Control (2026-08-24): `PASS`
  * Paired GNU Control (2026-08-23): `FAIL`

The paired GNU control run produced inconsistent results across dates (`2026-08-23=FAIL` vs `2026-08-24=PASS`). Converting a control-to-control disagreement or host-environment volatility into a product bug is explicitly prohibited under the review contract. The owner hypothesis of control/host instability was investigated against both the frozen/current Coreutils `nohup` binaries and the public suite-free s85 probe.

## Deterministic Behavior Isolation (s85 Probe Results)

Coreutils `nohup` was evaluated against all five core execution vectors using the public suite-free s85 probe and focused tests in `cmds/nohup`:

1. **0 / 126 / 127 Exit Status Partitioning:**
   - **0 (Success):** Executable commands complete normally and return their exact exit status (`nohup true` exits `0`).
   - **126 (Found but Not Executable):** A non-executable file found during PATH or relative lookup fails with diagnostic and exits `126` (`nohup ./blocked_cmd` exits `126`).
   - **127 (Internal Error / Not Found):** Missing utility operands (`nohup`), missing PATH executables (`nohup missing_cmd`), and redirect setup failures return `127` unconditionally.

2. **SIGHUP Disposition & Process Immunity:**
   - Invoked utilities inherit `SIGHUP` set to ignored via `/bin/sh -c "trap '' HUP; exec \"$@\""`.
   - The `nohup` process wrapper itself suppresses default termination via `signal.Notify(ch, syscall.SIGHUP)` while waiting for child execution, ensuring the waiting process wrapper survives `SIGHUP` delivered during execution.

3. **File Descriptors & Creation / Append Semantics:**
   - Terminal standard input is redirected to write-only `/dev/null` before command lookup.
   - Terminal standard output appends to `./nohup.out` in the working directory (created with mode `0600`), with fallback to `$HOME/nohup.out`.
   - Appending to existing `nohup.out` preserves pre-existing file content.
   - Terminal standard error follows redirected standard output, or appends to `nohup.out` when standard output is closed.

4. **PATH Search & Resolution:**
   - Command lookup via `lookCommand` evaluates `PATH` relative to `RunContext.Dir`.
   - Correctly distinguishes non-existent entries from non-executable files.

5. **Child Execution & Text Script (ENOEXEC) Fallback:**
   - Plain text scripts lacking a shebang (`#!`) are executed via `/bin/sh` fallback, preserving invocation environment and argument vectors.

## Test Coverage

Focused regression and s85 probe coverage in `cmds/nohup/nohup_s85_probe_test.go`:
- `TestS85ProbeExitCodes`: Pins exit statuses `0`, `126`, and `127`.
- `TestS85ProbeDescriptorAndAppendMode`: Pins `0600` file creation mode, descriptor redirection, and append behavior over pre-existing content.
- `TestS85ProbeWriteAndKillHangup`: Pins child process `SIGHUP` immunity and output persistence to `nohup.out`.

All `cmds/nohup` package tests pass 100%.

## Disposition & Matched-Replay Prerequisite

* **Disposition:** Control/Host Instability. No product-owned red exists in `cmds/nohup`.
* **Matched-Replay Prerequisite:** Re-certification of Profile D for `nohup:21` requires a stable host environment where the GNU control run deterministically passes without harness signal race conditions or PTY allocation variance between test runs.
