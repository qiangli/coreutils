# POSIX Go evidence closure: Profile C `cp`, `chown`, and `id`

Scope: `cmds/cp`, `cmds/chown`, and `cmds/id` against POSIX.1-2016
(Issue 7): [`cp`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html),
[`chown`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html),
and [`id`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html).
GNU-only options are retained extensions, not POSIX evidence.

## Historical diagnostic disposition

Earlier VSC-PCTS summaries retain aggregate diagnostic counts of 16 for
`cp`, 12 for `chown`, and 13 for `id`. The corresponding entry-level result
vector is not present in this repository, so this review cannot truthfully
identify each old diagnostic, reproduce it, or declare every entry closed.
The counts are therefore historical inputs only. They do not override current
source and focused-test evidence, and they must not be promoted to verified
closure until the manager reruns the external verification and reconciles its
individual results.

This audit instead records the accepted current fixes, the exact public tests
that exercise their behavior, and the integration boundaries that those tests
cannot close. No Bashy-owned source defect was reproduced in this review.

| Command | Current Profile C verdict | What remains outside focused closure |
| --- | --- | --- |
| `cp` | partial | Privileged device-node and foreign-ownership products; injected mid-copy read, write, and unlink failures; physical-symlink metadata outside Linux/Darwin; Windows runtime paths. |
| `chown` | partial | Privileged foreign UID/GID transitions and permission products; kernel/filesystem set-ID and special-file effects not safely exercised by an unprivileged test; runtime POSIX ownership semantics on non-Unix targets. |
| `id` | partial | A real set-ID process fixture; a conformant numeric-credential provider, or an explicit non-conformance disposition, on each non-Unix target. |

## `cp`

Accepted commits `014684e` (`fix(cp): enforce POSIX destination safety`),
`f523c76` (`fix(cp): preserve physical symlink metadata`), and `62cb891`
(`posix: correct filesystem batch review findings`) supersede substantial
current-source concerns without supplying an entry-by-entry mapping for the
historical count.

The current focused products include:

- destination identity, overwrite ordering, and extension non-interference:
  `TestCpInteractiveSameFileDiagnosesWithoutPrompt`,
  `TestCpUpdateDoesNotBypassSameFileDiagnostic`,
  `TestCpNoClobberDoesNotHideSameFile`, and
  `TestCpInteractiveDirDestDiagnosesWithoutPrompt`;
- safe directory and symlink destinations:
  `TestCpPhysicalSymlinkSameFileIsNotReplaced`,
  `TestCpPhysicalSymlinkDoesNotReplaceDestinationDirectory`,
  `TestCpRecursiveRejectsSymlinkAliasedDestinationInsideSource`, and
  `TestCpRecursiveRejectsDestinationAliasedToSourceSubdirectory`;
- preservation and fail-closed metadata handling:
  `TestCpPreservePhysicalSymlinkMetadataWithoutMutatingReferent`,
  `TestCpPreserveFailsLoudlyWhenAccessTimeIsUnavailable`,
  `TestCpPreserveModeAppliedAfterOwnership`, and
  `TestCpPreserveClearsSetuidWhenOwnershipFails`;
- invocation umask and long-path behavior:
  `TestCpRecursiveHonorsVirtualUmaskForAllCreatedTypes`,
  `TestCpRecursiveNewDirUmaskFinalMode`, and
  `TestCpNearPathMaxWithVirtualWorkingDirectory`.

Physical symlink creation and the Linux/Darwin symlink-metadata product do not
inherently require privilege. That case is distinct from device-node creation
and changing ownership to foreign IDs, which remain privilege/platform
integration boundaries. Unsupported metadata providers fail loudly rather
than reporting preservation that did not occur.

## `chown`

Accepted commit `8e92597` (`chmod/chgrp/chown: close POSIX Issue 7
interfaces`) and the detailed
[`chmod-chgrp-chown-s79.md`](chmod-chgrp-chown-s79.md) audit establish current
source and focused closure on supported Unix targets. The command is still
partial at the Profile C level because privileged and platform integration is
not equivalent to seam-backed or caller-owned-file evidence.

The current focused products include:

- required recursive traversal and symbolic-link selection:
  `TestChownTraversalModes`,
  `TestChownTraversalAndDereferenceAreOrthogonal`,
  `TestChownCommandLineLinkChainIsFollowed`,
  `TestChownSymbolicLinkTargetOfTheChange`,
  `TestChownSymbolicLinkCycleTerminates`,
  `TestChownMutualSymbolicLinkLoopTerminates`,
  `TestChownDanglingSymbolicLink`, and `TestChownCycleDiagnostic`;
- operand grammar and identity resolution:
  `TestChownNameIsPreferredOverNumber`,
  `TestChownReferenceIdsAreNotLookedUpAsNames`, and
  `TestChownNumericGrammarAndLookupFailures`;
- required syscall effects and native metadata identity:
  `TestChownUnchangedOwnershipStillCallsChown` and
  `TestChownNativeCtimeSymlinkAndHardLinkIdentity`;
- diagnostics, continuation, and status:
  `TestChownTransitionFailuresContinue`,
  `TestChownContinuesPastFailures`,
  `TestChownOutputFailureSetsStatusAndContinues`,
  `TestChownDoubleDashEndsOptions`, and `TestChownDashOperandIsAFileName`.

Root guards and GNU long options are extensions and are not used to claim
Issue 7 closure. On non-Unix builds `chown` is present but deliberately returns
a not-supported diagnostic and status 1; it does not silently map POSIX owner
IDs onto unrelated host attributes. That is a fail-closed implementation, not
a claim of POSIX runtime conformance.

## `id`

Accepted commits `49e8fab` (`id: handle output failures and expand operand
tests`) and `9561a21` (`id,logname: make POSIX evidence portable and explicit`)
are recorded in the detailed [`id-logname-issue745.md`](id-logname-issue745.md)
audit. They close the required current-source and focused behavior on supported
Unix targets without proving the 13 historical diagnostics individually.

The current focused products include:

- default real/effective fields and ordering:
  `TestIDDefaultReportsRealAndEffectiveWhenDifferent` and
  `TestIDDefaultOmitsEffectiveWhenEqual`;
- required selectors, name output, and live supplementary groups:
  `TestIDOnlyFlags`, `TestIDRealFlagWithOptions`,
  `TestIDRealAndEffectiveSelectors`, and
  `TestIDCurrentGroupsUseLiveProcessVector`;
- named-user default and every required selector/name combination:
  `TestIDNamedUserOperand` and `TestIDNamedUserOperandCombinations`;
- invalid combinations, lookup failures, and operand arity:
  `TestIDErrors` and `TestIDRejectsExtraUserOperand`;
- output errors and short writes: `TestIDOutputErrors`.

Real/effective divergence is tested through credential seams because an
ordinary test process is not set-ID. Native Unix builds supply process
credentials. Targets without POSIX numeric credentials remain best-effort or
explicitly unsupported and are not described as runtime-conformant. GNU-only
`-a`, `-p`, and `-z` behavior and translated diagnostic catalogs neither
establish nor block the required Issue 7 interface evidence above.
