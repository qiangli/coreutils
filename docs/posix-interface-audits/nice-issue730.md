# POSIX `nice` interface closure (issue 730)

This audit uses the Open Group POSIX.1-2016 Issue 7 `nice` utility page as the
normative reference. It supersedes the `nice` assessment in batch 6C and the
older batch-4 source review. The consolidated Sprint 79 ledger, generated
manifests, and shared command matrices are intentionally unchanged here.

## Clause map and evidence

- **Synopsis and option:** `nice [-n increment] utility [argument...]` is
  parsed until the first operand. Separate and attached `-n` accept signed
  decimal integers; invalid, missing, and unknown options return 125. `--`
  terminates options and a lone `-` is an operand. A missing utility returns
  125 whenever `POSIXLY_CORRECT` is present. GNU-only spellings and bare
  priority display remain compatibility extensions outside the valid Issue 7
  synopsis and are not conformance claims. Evidence:
  `TestParseNiceOptions`, `TestParseNiceInvalidAdjustment`,
  `TestParseNiceMissingArgument`, `TestParseNiceRejectsUnknownOptions`,
  `TestNicePOSIXModeRequiresUtilityOperand`, and the double-dash tests.
- **Default effect and privilege rule:** the implementation-defined default
  increment is +10. On Unix, a pure-Go re-exec trampoline blocks on a pipe
  while its parent attempts to set the helper PID's absolute nice value. The
  parent releases the helper only after that attempt, and the helper then
  overlays itself with the requested utility. Thus utility code cannot run
  before the adjustment attempt and an in-process embedding host is never
  re-prioritized. If the scheduler rejects the requested value, the parent
  emits the permitted warning and still releases the helper; the utility runs
  unchanged and its status remains authoritative. Evidence:
  `TestPriorityHelperWaitsForBarrierBeforeExec`,
  `TestNiceAdjustmentFailureStillInvokesUtilityAndUtilityStatusWins`,
  `TestNiceUtilityStartsAtAdjustedPriority`, and
  `TestNiceDoesNotAlterOwnPriority`.
- **Operands:** the resolved utility and every following argument, including an
  empty argument or an option-looking argument, are passed unchanged. Executable
  text files rejected with `ENOEXEC` are invoked through `/bin/sh`, matching
  POSIX utility search behavior. Evidence:
  `TestNicePassesUtilityArgumentsAndStandardStreamsUnchanged`,
  `TestPriorityHelperPreservesENOEXECScriptFallback`, and
  `TestNiceRunsExecutableTextWithoutShebang`.
- **Standard input and input files:** `nice` reads no standard input and opens
  no input file. The invoked utility receives the invocation's input stream
  unchanged. Evidence: `TestNicePassesUtilityArgumentsAndStandardStreamsUnchanged`.
- **Environment:** `PATH` is resolved from `RunContext.Env`, including the
  distinction between unset and empty PATH and relative/empty components. The
  complete invocation environment is passed to the utility; the internal
  trampoline uses only its inherited one-shot barrier descriptor and reserves
  no environment variable. Even a user variable matching the former internal
  marker is preserved exactly. `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`,
  and `NLSPATH` are therefore also inherited. Evidence:
  `TestNicePathUnsetFindsCommand`,
  `TestResolveCommandResolvesPathEntriesFromRunContext`,
  `TestPriorityHelperWaitsForBarrierBeforeExec`, and
  `TestNicePreservesEnvironmentThatCollidesWithFormerHelperMarker`.
- **Standard output, standard error, and output files:** `nice` writes no
  standard output and creates no output files. Utility stdout/stderr are passed
  through unchanged. `nice` itself uses stderr only for diagnostics, including
  the non-fatal scheduler warning. Evidence:
  `TestNicePassesUtilityArgumentsAndStandardStreamsUnchanged` and
  `TestNiceAdjustmentFailureStillInvokesUtilityAndUtilityStatusWins`.
- **Exit status and errors:** a successfully invoked utility's status is
  propagated. Signal termination maps to 128+signal while preserving the raw
  boundary signal in `RunContext.ExitSignal`. Lookup failure is 127,
  found-but-not-invokable and post-helper `exec` failures are 126, and parsing
  or failure to start `nice`'s own helper is 125. Evidence:
  `TestNiceCommandExitStatuses`, `TestNiceChildExitPropagates`,
  `TestNiceReportsSignalExitCode`, and
  `TestPriorityHelperExecFailuresUsePOSIXStatuses`.
- **Asynchronous events and consequences of errors:** no signal handlers are
  installed and no persistent process state is modified. Context cancellation
  remains attached to the helper PID and therefore to the same PID after
  `exec`.

## Implementation boundary

The pre-exec race recorded by the earlier audits is closed. The trampoline is
not a host shell-out: it re-executes the already-running pure-Go host, blocks
that child before user code, applies priority to its PID from the parent,
closes the one-shot barrier descriptor, and overlays the same PID with the
required utility operand. It consumes no user environment entry. `/bin/sh` is
used only for the POSIX `ENOEXEC` executable-text fallback.

On non-Unix targets there is no POSIX nice value. The utility is still invoked
and a deterministic unsupported-priority warning is written, rather than
claiming an adjustment occurred. On Unix, real negative adjustments remain
privilege- and scheduler-policy-dependent; the non-fatal failure ordering is
covered through a hermetic seam and positive adjustment is exercised against a
real child. A found-but-not-invokable state is not constructable on Windows,
where executability is extension-based.

Diagnostics are deterministic English. This repository does not ship
translated `LC_MESSAGES`/`NLSPATH` catalogs, so translated diagnostics remain
the only Issue 7 locale limitation. Locale variables are otherwise preserved
for the invoked utility.
