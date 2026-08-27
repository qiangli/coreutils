# `patch` continuation ledger

The pure-Go `patch` applet is the sole multicall owner. There is no external
provider definition, build recipe, cache entry, or fallback.

## Implemented lanes

- Unified, context, and normal diff parsing and application.
- Multi-file unified and context patches, including file creation and deletion.
- Forward and explicit reverse application, offset search, bounded context fuzz,
  already-applied detection, and reject-file output.
- Header path selection and stripping, explicit input/output/reject paths,
  directory selection, backups, empty-file removal, quiet mode, whitespace-aware
  matching, and dry runs.
- Atomic replacement of existing files while preserving their permission bits.
- `-e` scripts from `diff -e`, applied directly by the pure-Go engine.
- `-D define` conditional merges for insertions, deletions, and replacements.
- POSIX filename determination through old/new headers, `Index:`, common
  indentation removal, and a stdout prompt whose answer is read from the
  controlling terminal.
- Default reversed/already-applied detection with an LC_MESSAGES `yesexpr`
  prompt; explicit `-R` and `-N` retain their distinct meanings.
- `-b` overwrites a stale `.orig` but saves only the first pre-patch version
  per pathname; with `-o`, only a pre-existing output file is backed up.
- Multi-file `-o` concatenation, including successive intermediate versions
  when several patch portions target the same file.
- Rejects preserve unified input as unified; copied-context and normal input
  are emitted as copied-context, as required by Issue 7. Reversed rejects have
  swapped hunks and filenames.

The command and package tests cover parsing, all three textual formats, round
trips, path stripping, reverse application, creates/deletes, rejects, backups,
multi-file patches, missing final newlines, drift/fuzz, and unsupported binary
sections.

## Outside the base Profile D surface

- XSI-optional SCCS retrieval is not enabled; the GNU `-g` RCS/SCCS extension
  is accepted but fails closed.
- Binary patches and rename-only Git patches are reported, never silently
  treated as applied.
- GNU-only fuzz and whitespace behavior beyond the Issue 7 minimum remains a
  compatibility objective, not a certification requirement.

## Issue 7 completion checklist

| Clause | Focused evidence |
|---|---|
| four input forms and selectors `-c/-e/-n/-u` | `TestApplyContextFormat`, `TestRoundTripNormalDiff`, `TestEdFlagAppliesDiffEdScript`, `TestApplyEdDiffDotProtectionAndEmptyScript` |
| `-D define` | `TestIfdefMergeRetainsBothVersions`, `TestApplyIfdefInsertionDeletionAndReplacement` |
| `-b` first-copy and overwrite rules | `TestApplyBackupFlag`, `TestBackupOverwritesPreexistingOrigOnce`, `TestOutputBackupAndRejectNamesFollowOutput` |
| filename determination / indentation / prompt | `TestIndexSelectsNormalDiffTarget`, `TestIndexExistingTargetPrecedesCreationFallback`, `TestParseIndexAndCommonIndent`, `TestMissingHeaderTargetPromptsForFilename`, `TestFilenamePromptIsWrittenToStdout` |
| default reversal, `-R`, `-N` | `TestDefaultReversalPromptsAndAppliesReverse`, `TestCreationPatchAgainstPostimagePromptsAndRemovesOnReverse`, `TestAcceptedReversePersistsAcrossFollowingFilePortions`, `TestApplyReverseFlag`, `TestAlreadyAppliedRequiresForwardFlag` |
| `-o` concatenation/intermediate versions | `TestIndentedPatchAndMultiFileOutput`, `TestOutputConcatenatesIntermediateVersionsForSameFile`, `TestOutputCarriesNewlyCreatedFileIntoLaterPortion` |
| copied-context/unified rejects | `TestRejectPreservesInputNotation`, `TestApplyConflictWritesRejectAndExitsOne`, `TestReverseRejectSwapsHeadersAndHunk` |

The source-level POSIX interface is implemented. The manifest evidence state
remains **partial** until the independent Profile D integration replay supplies
the certification evidence; this ledger does not claim certification by itself.
