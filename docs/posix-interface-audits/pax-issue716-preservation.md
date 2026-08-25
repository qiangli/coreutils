# POSIX pax focused closure: `-p` preservation

This issue-716 audit covers only the POSIX.1-2016 Issue 7 `pax -p`
preservation grammar and its extraction/copy effects. The normative source is
the [Issue 7 pax utility](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html).
No shared TSV, matrix, or generated aggregate is changed here.

## Disposition

| Interface | Closed behavior | Exact evidence | Residual |
| --- | --- | --- | --- |
| Ordered `-p string` grammar | `a`, `e`, `m`, `o`, and `p` are processed character-by-character across repeated options. A later `e` re-enables all preservation; a later `a` or `m` disables the corresponding time. Empty and unsupported strings fail as usage errors. `-p` is accepted only in read and copy modes. | `TestPreservationGrammarOrderedAndRepeated`, `TestRepeatedPOptionsReachExtractionInCommandOrder`, `TestModeOptionLegality`. | Bashy defines no additional implementation-defined characteristic letters. |
| Extraction attributes | Access and modification times are restored by default when present. `o` restores uid/gid; `p` restores the archived mode; `e` requests all four groups. Without mode preservation, the archived permission mode is filtered through the invocation umask as for `creat()`. Ownership precedes mode, times are last, and set-ID is cleared unless ownership was requested and succeeded. Directories retain owner access while populated, then are finalized deepest-first; a later duplicate member supplies the final attributes. Symlink owner/time operations do not follow the link and no non-portable symlink chmod is attempted. | `TestExtractDefaultCreationAndPreservedMode`, `TestPreservationFailuresKeepFilesClearSetIDAndContinue`, `TestModeAndTimeFailuresKeepExtractedFile`, `TestDirectoryAttributesFinalizeAfterChildren`, `TestRestrictiveDirectoryModeIsDeferredUntilChildrenExist`, `TestDirectoryFinalizationUsesDepthAndLastDuplicate`, `TestSymlinkPreservationUsesNoFollowOwnerAndTimes`. | Linux and Darwin have independent `utimensat()` time slots and no-follow symlink handling. Windows refuses symlink-time preservation; other unsupported platforms refuse time or ownership preservation explicitly rather than reporting false success. Special-file extraction remains a separately tracked pax residual. |
| Copy mode and direct `-l` | Ordinary copy mode carries source ownership, mode, access time, and modification time through its internal pax archive and applies the same extraction policy. Direct `-l` retains a possible hard link; when a source has set-ID but ownership is not requested it instead copies to a new inode and clears set-ID. A hard-link fallback applies requested attributes and normal umask-filtered creation mode. Preservation failures diagnose, retain the destination, continue traversal, and make final status non-zero. Directory attributes are finalized after descendants. | `TestCopyLinkSetIDFallbackAndPreserveEverythingLink`, `TestCopyLinkFallbackAppliesRequestedAttributes`, plus the extraction-policy evidence above exercised by the ordinary copy lane. | A successful direct hard link necessarily shares its inode attributes with the source, as permitted by copy-mode `-l`. Filesystems that reject hard-linking a symlink use the existing copy fallback. |
| Preservation failures | Requested ownership, mode, and time operations are attempted independently. Any failure is diagnosed and produces non-zero final status without deleting the extracted/copied object or preventing later members from being processed. | `TestPreservationFailuresKeepFilesClearSetIDAndContinue`, `TestModeAndTimeFailuresKeepExtractedFile`. | Diagnostic localization is absent. |

## Gate scope

The focused `cmds/pax` tests include ordered/repeated parsing, exact umask and
mode behavior, uid/gid seams, set-ID safety, default and suppressed times,
directory ordering, symlink no-follow behavior, forced link fallback, and
injected preservation failures. Focused count, race, host vet, and supported
cross-platform compile/vet results are recorded with the issue commit.
