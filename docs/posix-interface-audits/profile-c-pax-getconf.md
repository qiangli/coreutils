# Profile C focused closure: pax and getconf

Status: **partial** for both commands. This audit is fail-closed: it records
only what was re-verified against the current source tree and the POSIX.1-2016
(Issue 7) utility clauses on this pass, labels the retained diagnostic counts
as unmapped historical artifacts, and records the focused repairs that closed
the two source-owned reconciliation candidates.

## Scope and method

- Source reviewed: `cmds/pax` (`pax.go`, `options.go`, `select.go`, `modes.go`,
  `walk.go`, `preserve.go`, `archive.go`, `list_format.go`, `interactive*.go`)
  and `cmds/getconf` (all non-test files).
- Specification text consulted: Issue 7 `pax` and `getconf` utility pages
  (OPTIONS, OPERANDS, STDIN, STDOUT, STDERR, EXTENDED DESCRIPTION for `-x
  format` defaults and list-mode formats; getconf OPERANDS/STDOUT/EXIT STATUS).
- Focused gates run on this branch (see Verification Evidence).

## Stale diagnostic: unmapped historical

The retained diagnostic — `pax` 249 TPs / 113 blockers and `getconf` 104 TPs /
26 blockers — predates the current tree. **No per-test ledger exists that maps
those test points or blockers to test identifiers, clauses, or commits**, so
they are recorded here as *unmapped historical* counts. They are neither
re-derived nor reconciled in this pass and MUST NOT be used to infer current
defects or current conformance percentages. Current main is far newer than the
artifact; the only admissible evidence for current state is the source and
focused tests cited below.

## Disposition

| Command | Issue 7 source | Verdict | Exact residual after this pass |
| --- | --- | --- | --- |
| `getconf` | [getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html) | partial | Utility-syntax surface (arity, `-v` handling, unknown-name vs `undefined`, exit codes) re-verified against source and focused tests. Inventory counts re-measured (see below) and the prior doc's figures corrected. libc-owned values (`PATH` on Linux, `RE_DUP_MAX`, `SYMLOOP_MAX`, non-target programming environments) remain a platform/integration boundary: Linux exposes no kernel API for libc policy. `getconf` carries no open source-owned candidate from this pass, but the row stays partial because the historical 104/26 ledger was not re-derived and host-differential coverage exists only on Darwin/Linux test hosts. |
| `pax` | [pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html) | partial | Four modes, option legality matrix, `-b` grammar, `-s` first-success semantics, selector `-c/-n/-d`, `-p` ordered grammar and set-ID rule, extended-header `-o` grammar/precedence, transcode matrix, traversal `-H/-L/-X` with cycle termination, and copy-mode `-l`/`-t` all re-verified in source and covered by focused tests. The two source-owned reconciliation candidates found by this pass are now repaired and regression-tested (see next section). The historical 113-blocker ledger was not re-derived, so this row remains partial. |

## Closed source-owned reconciliation candidates

1. **Default physical blocking for `-x pax`.** Issue 7 `-x format` states the
   default blocksize for the *pax* format for character special archive files
   is 5120 (ustar 10240, cpio 5120). `defaultBlockSize`
   (`cmds/pax/pax.go`, `defaultBlockSize`) formerly returned 10240 for both
   tar-derived formats regardless of sink. The write path now inspects the
   actual selected sink after open—including stdout and `-f -`—and chooses
   5120 for character-special pax/cpio sinks while preserving 10240 for ustar;
   explicit `-b` remains authoritative. See `pax-issue776-blocksize.md` and
   `cmds/pax/issue776_blocksize_test.go`.
2. **Verbose list output for hard links to previous members.** Issue 7 STDOUT
   requires, in list mode with `-v`, `"%s==%s\n", <ls -l listing>, <linkname>`
   for pathnames representing hard links to previous members of the archive.
   The listing path now emits that marker and resolves link targets by archive
   occurrence, including interactive rename, substitution, duplicate raw
   names, and post-substitution collisions. See
   `pax-issue775-hardlink-list.md` and `cmds/pax/list_hardlink_test.go`.

Both source-side candidates are closed by focused implementation and tests.
They no longer justify a current Bashy-source blocker; `pax` remains partial
for the explicitly attributed integration/evidence boundaries below.

## Verified evidence and exact mappings

### `getconf`

Commits on this branch's history implementing the current surface (verified
via `git log -- cmds/getconf`, oldest first):

- `62de728` initial Go implementation
- `8689d07` POSIX utility limits
- `93192bc` platform-derived values
- `82ede0f` fail closed for unavailable platform values
- `4f261db` reject empty POSIX specification
- `ceae89d` close POSIX configuration inventory gaps
- `2fbc91c` support Linux LP64 environments

Focused tests (`cmds/getconf/getconf_test.go`, `linux_posix_test.go`), with
the TestIDs verified present and passing on this pass:

- Utility syntax and arity: `TestPOSIXArityAndOptionForms`,
  `TestMissingOperand`, `TestAllListsAndDoesNotTakeOperands`,
  `TestUnsupportedSpecificationIsRefused`
- Name-vs-undefined semantics: `TestUnknownVariableIsAnErrorNotUndefined`,
  `TestPathErrorsWriteNoStdoutAndFail`, `TestStandardOutputFailureIsAnError`
- Inventory integrity: `TestMandatoryTableInventoryAndCompatibilityAliases`,
  `TestEveryInventoryNameHasAValueClass`,
  `TestCompileTimeMinimumsComeFromTheStandard`
- Host differentials (platform evidence, Darwin/Linux hosts only):
  `TestAgreesWithSystemGetconf`, `TestPathconfAgreesWithSystem`,
  `TestDarwinConfstrAdapterMatchesEveryQueryableValue`,
  `TestDarwinAdapterMatchesEverySafelyQueryableValue`,
  `TestDarwinRegressionValues`, `TestLinuxReportsOnlyDerivedRuntimeValues`,
  `TestLinuxDerivedValuesMatchHostGetconf`,
  `TestLinuxDerivedPathValuesMatchHostGetconf`,
  `TestLinuxTimestampResolutionIsExactOrUndefined`,
  `TestLinuxProgrammingEnvironmentMatchesUbuntuOracle`,
  `TestLinuxDoesNotClaimOtherProgrammingEnvironments`, `TestWindowsFailsClosed`

Inventory counts re-measured against the current tree (throwaway probe,
removed after measurement): `systemInventoryNames` = 166, resulting `sysVars`
= 222 (including compatibility aliases and platform locals), `pathVars` = 21,
`confstrVars` = 31, `standardMinimumValues` = 50. **This corrects the prior
revision of this document, which claimed "122 mandatory variables, 165 total
in system inventory" — those figures no longer match the code.**

### `pax`

Commits on this branch's history (verified via `git log -- cmds/pax`, oldest
first; the `issue715/716/717` sprints and later corrections):

- `ecc5845` wave B archive-lane corrections
- `9b71c2c` hardlink updates and append blocking
- `8784884` `-H`/`-L`/`-X` over one shared source walker
- `41a8518` POSIX traversal error handling
- `44de364` list error corrections
- `b614e20` interactive links and atime reset
- `27c4c30` concurrent mtime changes with `-t`
- `8b7c051` POSIX preservation policy
- `6cd55bc` POSIX extended-header options
- `af1fd00` empty list format state
- `ddbe753` complete list time formats
- `7fff4b6` list option conformance gaps
- `dd63476` invalid rename terminal access
- `3aefbac` extended header text transcoding
- `9ef3a43` extended header error stabilization

Focused test files: `pax_test.go`, `issue715_test.go`,
`issue715_pty_unix_test.go`, `issue716_test.go`, `issue717_test.go`,
`wave_test.go`, `wave_b_test.go`, `archive_lane_test.go`, `follow_test.go`,
`list_io_test.go`. All TestIDs in those files were enumerated and the packages
pass with `-count=1` on this pass. Representative clause mappings verified:

- Option legality matrix (mode-restricted `-p`, `-b`, `-l`, `-t`, `-X`;
  `-c`+`-n` rejection; last-of `-H`/`-L` wins): `TestModeOptionLegality`,
  `TestFollowOptionsAreNoOpsInListAndRead`,
  `TestLastOfRepeatedFollowOptionsWins`
- `-b` grammar and physical blocking (factors, 512-multiple, 32256 cap,
  defaults per format, append alignment): `TestBlockSizeGrammar`,
  `TestBlockSizeMultiplicationIsChecked`,
  `TestPhysicalBlockingDefaultsAndExplicitSizes`,
  `TestWriteDefaultIsPhysical`
- `-s` ed-style semantics (any delimiter, first successful substitution wins,
  `g`/`p` flags, empty result drops the member): `TestSubstitutionAcceptsAlternateDelimiters`,
  `TestSubstitutionGlobalFlag`, `TestSubstitutionPrintReportsRename`,
  `TestSubstitutionToEmptyDropsTheMember`
- Selection `-c`/`-n`/`-d`/hierarchy and unmatched-pattern diagnostics:
  `TestPatternSelectionAndComplement`, `TestSelectorDashNFirstMatchOnly`,
  `TestSelectorDashDStopsHierarchy`, `TestUnmatchedPatternIsDiagnosed`
- `-p` ordered grammar, set-ID suppression rule, failure diagnostics,
  deepest-first directory finalization: `TestPreservationGrammarOrderedAndRepeated`,
  `TestPreservationFailuresKeepFilesClearSetIDAndContinue`,
  `TestDirectoryAttributesFinalizeAfterChildren`
- `-o` grammar/precedence/transcode matrix and `invalid=` actions:
  `TestPAXOptionGrammarWhitespaceEscapedCommaAndTrailingComma`,
  `TestPAXReadAndListHeaderPrecedence`, `TestPAXInvalidActionTranslationMatrix`,
  `TestPAXUnknownHeaderCharsetFailsClosed`
- Traversal `-H`/`-L`/`-X`, cycle termination, copy `-l`/`-t`:
  `TestDashHFollowsCommandLineSymlinksOnly`,
  `TestDashLFollowsEncounteredSymlinks`, `TestFollowCycleTerminatesPax`,
  `TestDashXPrunesOtherDeviceDirectories`, `TestCopyLinkRegularAndSymlinkFollowModes`,
  `TestResetAccessTimeDoesNotRollBackConcurrentMtimeChange`
- cpio octet format, hardlink identity, trailer/escape safety:
  `TestCPIOFormatWritesPOSIXArchive`, `TestODCIdentitiesAreOneToOne`,
  `TestNewcHardlinkDataOnAnyMemberMaterializesEveryName`,
  `TestCPIOEscapeAttemptsAreRefusedAtomically`

## Residual attribution summary

- **Unmapped historical**: the retained `pax` 249/113 and `getconf` 104/26
  counts (see Stale diagnostic section). Not evidence of anything current.
- **Platform/integration boundaries** (carried forward from prior audits, not
  re-derived here): libc-policy variables on Linux without a C toolchain;
  `getconf` message localization; `pax` special-file creation subject to host
  privileges; filesystem timestamp-resolution quantization where `utimensat`
  semantics differ.
- **Source-owned open items**: none identified by this pass after the two pax
  repairs above.

## Verification evidence (this pass)

Executed on this branch, darwin/arm64:

- `go build ./cmds/pax ./cmds/getconf` — clean.
- `go test -count=1 ./cmds/pax ./cmds/getconf` — both packages pass
  (uncached).
- `go vet ./cmds/pax ./cmds/getconf` — clean.

The original audit pass made no source changes. The two candidates it isolated
were subsequently implemented and covered by focused tests; this document now
records that disposition without treating the unmapped historical counts as
current failures.
