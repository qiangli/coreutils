# POSIX `newgrp` interface closure (issue 731)

This audit uses the Open Group POSIX.1-2016 Issue 7 `newgrp` utility page as
the normative reference. It re-audits the implementation after the later
credential, supplementary-group, login-environment, password-prompt, and
capacity-fallback changes, and supersedes the older `newgrp` assessments in
`go-batch-4.md` and `go-evidence-closure-batch-6c.md`.

## Clause map and evidence

- **Synopsis and options:** `newgrp [-l] [group]` accepts `-l`, stops option
  processing at `--`, and rejects unknown options or more than one operand
  without starting a shell. The result of a first argument exactly equal to
  `-` is unspecified by Issue 7. `--login`, help, and version spellings are
  non-colliding extensions. Evidence: `TestUsageErrorsStartNoShell`,
  `TestHelpAndVersionStartNoShell`, `TestLoginShellArgv0AndDirectory`, and the
  parser exercised throughout `newgrp_test.go`.
- **Group operand and no-operand behavior:** a name lookup always precedes
  numeric interpretation. A digits-only non-negative numeric operand is
  interpreted by value, including leading-zero spellings. With no operand,
  the password-database primary GID becomes both real and effective GID and a
  fresh `GroupsForUser` query rebuilds the supplementary list. Evidence:
  `TestNumericOperandPrefersTheGroupName`,
  `TestNumericOperandUsesTheNonNegativeGIDValue`,
  `TestNoOperandRevertsToThePrimaryGroup`, and
  `TestNoOperandDatabaseGroupFailureStillLaunchesUnchanged`.
- **Membership and passwords:** primary-group, current supplementary-group,
  and named member-list membership each permit the change without a prompt.
  A non-member with a usable group password is challenged; locked or absent
  passwords are denied under the implementation-defined no-password policy.
  An unreadable shadow database or unsupported hash is never mistaken for
  successful authentication. Evidence: `TestAuthorize`,
  `TestMemberOfAPasswordedGroupIsNotChallenged`, the password success/failure
  tests, and the independent crypt vectors in `crypt_test.go`.
- **Password input, standard input, and standard error:** password input opens
  `/dev/tty` and disables echo; it never consumes `RunContext.In`. The prompt
  and trailing newline use standard error as required. A missing controlling
  terminal denies the change and still launches the shell unchanged. Evidence:
  `TestPasswordPromptUsesStderrAndReadsControllingTTY`,
  `TestPasswordPromptDoesNotClaimATerminalOnOpenFailure`, and
  `TestPromptFailureIsDeniedNotAssumed`.
- **Credential and supplementary-group effects:** the credential plan carries
  equal real and effective target GIDs and the complete supplementary list.
  It implements both Issue 7 branches based on whether the old effective GID
  is in the list. Best-effort additions respect a known capacity, and an
  `EINVAL` discovered by the kernel retries without only the optional append;
  mandatory deletions and GID assignments remain. The Unix adapter maps the
  plan to `syscall.Credential` with `setgroups` enabled. Evidence:
  `TestSupplementaryGroupChangeRules`,
  `TestSyscallCredentialImplementsThePlan`,
  `TestSupplementaryCapacityFallbackRetainsMandatoryGIDPlan`, and
  `TestSupplementaryCapacityFallbackIsNarrow`.
- **Failure to change group:** authorization, database, current-credential,
  and kernel assignment failures diagnose but do not prevent shell creation.
  The retry carries no credential request, so the inherited group state is
  unchanged. Login-shell construction survives the retry. Evidence:
  `TestRefusedChangeStillStartsTheShellWithTheGroupUnchanged`,
  `TestCurrentCredentialReadFailureStillLaunchesUnchanged`,
  `TestKernelRefusalRetriesWithoutTheCredential`, and
  `TestLoginEnvironmentAndStatusSurviveKernelAssignmentFailure`.
- **Shell environment:** a normal invocation preserves the working directory,
  exact exported environment, standard streams, and file-creation mask. For an
  embedded shell whose umask is virtual, a pure-Go child trampoline applies
  that mask immediately before overlaying itself with the required shell; the
  embedding process is not modified. `-l` supplies a login argv[0], home
  directory, clean login base environment, and retained terminal, timezone,
  and locale channel variables. Evidence:
  `TestNonLoginShellArgv0AndDirectory`, `TestLoginEnvironment`,
  `TestDefaultSpawnShellPreservesArgumentsDirectoryEnvironmentAndIO`, and
  `TestDefaultSpawnShellAppliesRunContextVirtualUmask`. The internal boundary
  is pinned by `TestRunUmaskHelperRequiresCompleteControlAndPreservesExecInputs`
  and `TestRunUmaskHelperRejectsInvalidControlOrMaskWithoutExec`.
- **Output files and asynchronous events:** `newgrp` creates no output files
  and installs no signal handlers. Output belongs only to diagnostics, the
  password prompt, and the required shell's inherited streams.
- **Exit status:** once a shell is created, its ordinary exit status is
  returned whether the group changed or not. Signal termination returns
  `128+signal` and records the raw boundary signal; a reused `RunContext` is
  cleared before every invocation. If no shell is created, the result is
  greater than zero. Evidence: `TestSuccessfulGroupChangePropagatesShellStatusThroughSpawnSeam`,
  `TestDefaultSpawnShellPropagatesExitStatus`,
  `TestDefaultSpawnShellPropagatesSignalStatus`,
  `TestNormalRunClearsAReusedContextSignal`, and
  `TestShellThatCannotStartIsAnError`.
- **Environment and locale:** non-login invocations preserve every exported
  entry. Login reconstruction preserves the Issue 7 locale variables,
  including `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and `NLSPATH`, for
  the new shell and for its diagnostic locale selection.

## Honest implementation boundary

The repository runs applets in-process, so `newgrp` starts and waits for a
child shell instead of replacing the embedding host. Issue 7 explicitly allows
implementations where `newgrp` is invoked as a subprocess; shell status and
signal information are propagated across this boundary.

The test suite does not attempt a real setuid-root credential transition. It
proves the complete credential plan and the Unix syscall adapter without
changing the test runner, while real unprivileged kernel refusal is handled by
the required diagnostic-and-unchanged-shell path. Real NSS/PAM policy,
directory-service availability, and an attended controlling-terminal password
entry are host facilities outside hermetic coverage. The TTY descriptor,
no-echo reader, prompt channel, and all authorization outcomes are tested
through privilege-contained seams.

The exact breadth of a login environment and additional authorization policy
is implementation-defined. This implementation documents and tests its clean
base. Modern password schemes not implemented by the pure-Go verifier are
refused explicitly rather than approximated. Windows has no POSIX group ID and
fails loudly instead of claiming success. Diagnostics are deterministic
English because the repository ships no translated `LC_MESSAGES`/`NLSPATH`
catalogs; locale variables are nevertheless retained for the required shell.

Issue 7 does not specify how the shell path is selected and does not require an
`execvp()` command-search fallback for an explicitly selected shell. Therefore
an unusual `$SHELL` or password-database shell that is an executable text file
without a valid executable format fails with a diagnostic and status greater
than zero; `newgrp` does not silently replace that selected shell with
`/bin/sh`.
