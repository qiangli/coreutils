# POSIX pax focused closure: interactive names, copy links, source atimes

This issue-715 audit covers only three confirmed POSIX.1-2016 Issue 7 gaps in
`pax`: `-i`, copy-mode `-l`, and write/copy `-t`. The normative source is the
[Issue 7 pax utility](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html).
No shared TSV or generated aggregate is changed here.

## Disposition

| Interface | Closed behavior | Exact evidence | Residual |
| --- | --- | --- | --- |
| `-i` | One read-write controlling-terminal session is opened before mode work. Every selected list/read/write/copy member is prompted after `-s`; blank skips, `.` retains the substituted name, and another complete line renames it. Open/read/write/EOF/close failures diagnose and produce non-zero status. Read mode resolves all answers before extraction, preventing a terminal failure from leaving a partial tree, and renamed hard-link targets follow their renamed member. Copy mode prompts only on its write side. | `TestInteractiveListOrdersSubstitutionBeforeResponses`, `TestInteractiveReadPreflightsAndRenamesHardlinks`, `TestInteractiveWriteAndCopyPromptExactlyOnce`, `TestInteractiveAndResetAccessTimeInCPIOWriteLane`, `TestInteractiveFailuresAreImmediateAndNonzero`, `TestInteractiveRenameUsesTheRealControllingPTY`. | Unix uses `/dev/tty`. Platforms without that interface fail explicitly rather than using standard input/output. Prompt localization is absent. |
| copy `-l` | Each transformed safe destination is hard-linked to the selected source object whenever possible. Default traversal links a symlink directory entry itself; `-H` follows a command-line symlink and `-L` follows encountered symlinks. Collision checks compare directory entries with `Lstat`, so distinct symlinks to one referent are not mistaken for one file. A hard-link failure falls back to ordinary regular-file or symlink copying. Existing `-k`/`-u`, traversal-cycle, verbose, interactive, substitution, and destination-safety decisions remain active. | `TestCopyLinkRegularAndSymlinkFollowModes`, `TestCopyLinkFallsBackAndKeepsDestinationSafe`, `TestCopyLinkDistinctSymlinksToSameReferentAreNotSameFile`. | Directory hard links are prohibited by filesystems and therefore use directory creation. Non-Unix platforms use their native `os.Link`; unsupported symlink hard links take the documented copy fallback. Special file types remain the separately tracked pax residual. |
| write/copy `-t` | Before reading each source regular file, directory, or symlink, pax captures its access time; restoration occurs after the callback and, for directories, after descent. Linux and Darwin use `utimensat()` with `UTIME_OMIT` for mtime, so a concurrent legitimate mtime update is never rolled back. Restoration failures are diagnosed and make final status non-zero. Linux and Darwin restore symlink timestamps without following; followed `-H`/`-L` paths restore the referent. | `TestResetAccessTimesWriteAndCopyAndFailureStatus`, `TestResetAccessTimesForDirectoriesAndSymlinks`, `TestResetAccessTimeDoesNotRollBackConcurrentMtimeChange`. | Windows exposes and restores regular/directory access times while preserving the mtime observed immediately before restoration, but reports unsupported symlink restoration. Platforms without a usable access-time field fail explicitly. Filesystem timestamp quantization remains a property of the backing filesystem. |

## Gate scope

The focused `cmds/pax` suite includes real-PTY, injected terminal-failure,
filesystem identity, forced hard-link fallback, safety rejection, timestamp,
and injected restoration-failure lanes. Count-10, race, host vet, and
Windows/Linux/Darwin cross-vet results are recorded with the issue commit.
The repository-wide `scripts/crossvet.sh` stopped at its pre-existing "applet
matrix was stale" aggregate precheck before target vet; this focused issue does
not regenerate shared artifacts. Shared aggregate manifests are intentionally
left for the integration owner.
