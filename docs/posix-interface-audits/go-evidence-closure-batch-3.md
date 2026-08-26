# POSIX Go evidence closure: owned batch 3

This audit covers `chgrp`, `chmod`, `chown`, `cp`, `ln`, `mkdir`, `mkfifo`,
`mv`, `rm`, and `tail` against POSIX.1-2016 (Issue 7). The normative sources
are the Open Group command pages linked below. GNU behavior is not used as
POSIX evidence.

The ledger rows now carry the exact applicable synopsis, options, operands,
traversal/token semantics, stream behavior, side effects, status contract, and
command-specific test references. All ten rows are deliberately `partial`, not
`verified`: every page carries `LC_MESSAGES` and the XSI `NLSPATH` catalog
behavior, and none of these packages implements translated diagnostics or
message catalogs (the `-i` prompt family additionally answers only the
C-locale `yesexpr` plus one provisioned `de_DE` table, refusing other locales
loudly). The command-specific residuals below make the remaining boundary
explicit.

## Verdicts and exact residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `chgrp` | [Issue 7 chgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html) | partial | Translated diagnostics absent. Real-syscall coverage is bounded by privilege: only a change to the caller's own primary group exercises `chown(2)`/`lchown(2)` end to end; every cross-group scenario — including which of chown/lchown each file in a `-H`/`-L`/`-P` walk receives — is evidenced through the `changeGroup` seam. Set-group-ID clearing is proven only for a regular file by an unprivileged caller; the non-regular-file "may be cleared" clause, privileged-caller behavior, and the ctime update are kernel-delegated and untested. POSIX leaves the `-R` default among `-H`/`-L`/`-P` unspecified; the implementation pins `-P` and the tests assert that pin, not a POSIX requirement. Extensions beyond the synopsis: long spellings, `-v`/`-c`/`-f`/`--quiet`, `--preserve-root`, `--reference`, `--from` (a uutils-shaped flag GNU chgrp does not ship), `--dereference`, `-h` combined with `-R`, `-H`/`-L`/`-P` accepted-and-ignored without `-R`, and options parsed after operands. The `!unix` arm refuses loudly; its Windows-tagged test is compiled by the cross-OS gate but not executed here. |
| `chmod` | [Issue 7 chmod](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html) | partial | Translated diagnostics absent. This batch fixes three defects: an octal mode is set absolutely in POSIX mode (`POSIXLY_CORRECT` present) instead of unconditionally applying the GNU keep-directory-setid rule for fewer than five digits (which remains the recorded non-POSIX default per `docs/reference-policy.md`); in POSIX mode `X` now evaluates the Issue 7 "current (unmodified)" mode — the mode before the invocation — while non-POSIX mode retains GNU 9.11's in-progress-mode behavior; and `--reference` copies RFILE's mode exactly instead of leaking a target directory's setgid bit through the short-octal rule. Remaining: Issue 7's page says nothing about symlinks and the default `-R` skips a command-line symlink-to-directory operand entirely (exit 0) — a GNU-shaped choice recorded, not required; `-v`/`-c`/`-f`/`--reference`/`--preserve-root`/`-H`/`-L`/`-P`/`--dereference`/`--no-dereference` are extensions beyond `[-R]`; only the first dash-prefixed mode argument is rescued (`chmod -w -x f` is a usage error where GNU accumulates); permcopy reads the in-progress mode across clauses (Issue 7's "current permissions" is ambiguous; matches gnulib); ctime and the implementation-defined set-id pass-through on non-regular files are kernel-owned and unprobed; the Windows build refuses loudly with exit 1. |
| `chown` | [Issue 7 chown](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html) | partial | Translated diagnostics absent. Same privilege boundary as chgrp (self-chown is the only real-syscall path; every foreign-uid/gid case including all `-H`/`-L`/`-P` × `-h` products is seam-evidenced via `changeOwner`), same kernel-delegated set-ID clearing limits and untested ctime, same pinned-not-required `-P` default, and the same beyond-synopsis extension surface. chown-specific: the colon forms `:group`, `owner:`, `:` and the empty spec are GNU extensions past the POSIX `owner[:group]` operand — POSIX's dot-separator latitude is deliberately not implemented; `numeric-owner:` where the uid resolves in no database is refused as invalid spec, a branch untestable on hosts whose uid always resolves; a numeric spec tolerates a leading `+` (`strconv.Atoi`), an unpinned corner POSIX does not address; `-R -H` naming a symlink-to-non-directory operand is unexercised. |
| `cp` | [Issue 7 cp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html) | partial | Translated diagnostics absent; the `-i` affirmative rule is C-locale `yesexpr` plus the provisioned `de_DE` catalog, other locales refused loudly. This batch fixes three defects: description step 1 now precedes step 3's prompt (`cp -i same same` previously prompted and, on decline or EOF, exited 0 with no diagnostic — the required same-file diagnostic was silently skippable); the GNU `-u` extension no longer bypasses that same-file diagnostic; and `-p` now duplicates ownership before mode and clears S_ISUID/S_ISGID when uid/gid duplication fails (previously the bits were chmod'd first and left set after a failed chown — the security-relevant inverse of the shall; the ordering half also means kernels that strip setuid on chown no longer lose the bit on a successful `-p`). The later Issue 779 closure keeps `-i` and `-f` independently effective whenever `POSIXLY_CORRECT` is present: a negative response skips the copy in either option order, before `-f` can matter to a failed destination open. GNU's final-option override remains unchanged outside POSIX mode, and the `-n` extension remains ordered against both. Evidence is the public path `TestCpInteractiveDecline/posix_force_and_interactive_are_independent` paired with `/gnu_last_option_wins_outside_posix_mode`. Remaining: a large documented GNU option surface is accepted beyond the three-form synopsis; mid-copy read/write error diagnostics, the `-f` path where the unlink itself fails, `--` termination, and the `.`/`..` step-2.b skip are untested; ownership-failure evidence runs through the `chownFn` seam and device-node duplication is untested (mknod needs root), while FIFO and Unix-socket duplication have real filesystem tests; on Windows special files refuse loudly with no runtime test; on `!linux/!darwin/!windows` the `-p` atime source falls back to mtime; a destination symlink-to-directory inside a `-R` hierarchy is judged by Lstat; `copySymlink` overwrite has no same-file guard. Declined-`-i` exit 0 and the `-R`-without-token physical default are deliberate pinned readings of Issue 7 latitude. |
| `ln` | [Issue 7 ln](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html) | partial | Translated diagnostics absent. This batch fixes the implementation-defined `-L`/`-P` default on the supported Unix syscall targets: the default hard-link path used `os.Link`, and darwin's `link(2)` follows symlinks while Linux's does not, so `ln sym out` silently produced `-L` behavior on darwin and `-P` on Linux; those targets now pin `-P` via linkat without AT_SYMLINK_FOLLOW. Remaining: other platforms fall back to `os.Link`, so their default link-to-symlink behavior is platform-defined and unproven; the single-operand `ln TARGET` form, `-t`/`-T`/`-b`/`-S`/`-i`/`-n`/`-r`/`-v`, long spellings, and the `SIMPLE_BACKUP_SUFFIX` read are GNU extensions beyond the synopsis; `-i` reads stdin and `-v` writes stdout where POSIX says "not used"; the last-of-`-f`/`-i` and last-of-`-L`/`-P` arbitration rescans raw argv, so a long-option abbreviation or a flag-shaped option argument can mis-arbitrate (extension-only surface; POSIX shorts unaffected); `-f` when removal itself fails and hard-link-to-directory refusal are exercised only incidentally; symlink tests skip where symlinks are unavailable. |
| `mkdir` | [Issue 7 mkdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html) | partial | Translated diagnostics absent. This batch fixes the trailing-slash `-p` defect: `mkdir -p [-m mode] dir/` (and `dir/.`) previously created the final directory as an intermediate — it received the `(S_IWUSR|S_IXUSR|~filemask)&0777` intermediate mode and `-m` was silently never applied; the operand is now separator-trimmed before the `-p` walk, pinned under a controlled umask. Remaining: on Windows `-m` is a loud documented refusal (no POSIX mode bits), so the mode surface is unix-only; symbolic `=` clauses start from `a=rwx` like `+`/`-` (the page pins only `+` and `-` to that assumed initial mode); `-v`, `-Z`/`--context`, and long spellings are extensions beyond the synopsis; diagnostics carry GNU-shaped wording the standard leaves unspecified. |
| `mkfifo` | [Issue 7 mkfifo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html) | partial | Translated diagnostics absent. The `-m` chmod grammar is evidenced across octal (incl. special bits), symbolic `+`/`-`/`=`, permcopy, `X`, `s`, `t`, and the assumed `a=rw` initial mode; omitted-who clauses respect the umask for `+`, `=`, and `-`, and both the virtual (RunContext) and process umask paths are pinned for the default mode. Remaining: `-Z`/`--context` is an accepted deterministic no-op extension (a real SELinux context is never set — pinned as no-op by test and recorded as the one accepts-without-doing surface); the invalid-mode diagnostic does not name the offending string; missing operand exits 1 with the GNU message rather than the repo's usage-error 2 (POSIX-conformant either way, pinned by the pre-existing test contract, flagged as a repo-convention divergence); the Windows loud-failure branch is compiled by the cross-OS gate but not executed here; whether setuid/setgid stick on a FIFO is filesystem-dependent and asserted only on the host filesystems. |
| `mv` | [Issue 7 mv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mv.html) | partial | Translated diagnostics absent; prompt matching is C-locale `yesexpr` plus the carried `de_DE` table, failing closed otherwise. The "standard input is a terminal" condition of the implicit unwritable-destination prompt is real (`term.IsTerminal`) but provable only through the injected `moverDeps` seam — no pty exists in the hermetic suite. This batch fixes rename() equivalence for an existing empty destination directory by using direct `syscall.Rename` on unix; fixes same-file identity to compare final directory entries rather than following symlink referents; and prevents the GNU `-u` extension from silently bypassing same-file diagnostics. Windows MoveFileEx cannot replace an existing directory and remains a loud platform residual. Unlike cp, mv's POSIX step order puts the prompt (step 1) before the same-file rule (step 2), and the same-file outcome takes the diagnostic-and-status branch of the Issue 7 alternatives — both pinned by test; `-u` has its GNU-compatible pre-prompt diagnostic. Remaining: the GNU option surface beyond `[-if]` is accepted as documented extensions; EXDEV duplication of FIFO/device nodes exists but is untested (mknod needs privileges); ACLs/xattrs are not duplicated in the fallback (implementation-defined latitude); exit 0 when every move was skipped by a non-affirmative reply reads "moved successfully" as excluding operator-canceled operands. |
| `rm` | [Issue 7 rm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rm.html) | partial | Translated diagnostics absent; same C-locale `yesexpr` and seam-only terminal condition as mv. This batch extends the identity-checked root refusal to every path that attempts a removal — previously `-r`/`-R` only, now also the `-d` extension (pre-fix, `rm -d` on a root-resolving operand actually removed it under the injected guard) — and stops the implicit write-protection prompt from firing for symlink operands: unlink() removes the link itself, whose permissions never deny writing, while the previous `access(2)` check consulted the referent and prompted for dangling links where POSIX/GNU/BSD do not (`-i` still prompts for links). A bare `rm /` stops at the `Is a directory` diagnostic without any removal attempt — outcome-conformant with the 2016-edition root refusal though without a dedicated root diagnostic; `--no-preserve-root` deliberately bypasses the refusal as the documented GNU extension spelling, a knowing deviation from the unconditional Issue 7 rule. Remaining: the last-occurrence `-f`/`-i` override is resolved textually and does not recognize abbreviated long spellings (`--for`, `--inter`), so those extension spellings can defeat last-wins while exact long forms and all short clusters are correct; `-I`, `-d`, `-v`, `--one-file-system`, and `--progress` are extensions beyond `[-iRr]`. |
| `tail` | [Issue 7 tail](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html) | partial | Translated diagnostics absent. The load-bearing clauses are implemented and pinned: default `-n 10`; the `+`/`-`/unsigned sign semantics at origin 1 for `-c` and `-n`, including the `-c +N` read-driven skip on non-seekable input; stdin when no operand; `-f` never terminating at EOF (pinned deterministically via context cancellation and a FIFO-across-writers test); and the rule that `-f` is ignored when no file operand is given and stdin is a pipe or FIFO — `tail -f` and `tail -f -` on a pipe do a single pass and exit 0, while a regular file on stdin is followed by descriptor. Residuals: multiple file operands, headers, `-F`/`-q`/`-v`/`-z`/`--pid`/`--retry`/`--sleep-interval`/`--max-unchanged-stats`/`--debug`, multiplier suffixes, and the obsolescent leading `-NUM` rewrite are extensions beyond the one-file synopsis; under `-f` multiple operands are followed sequentially, not concurrently (a recorded extension deviation from GNU); `-c` with `-n` resolves last-one-wins instead of an exclusivity error; `scanOrder` misses the mode letter in clustered args like `-fn2` (matters only inside that extension combination); descriptor-mode truncation is silent (old offset retained — POSIX leaves shrink/rename unspecified); a character-device stdin under `-f` takes the ignore path within the page's latitude; pre-Flush write errors are labeled "error reading" (wording unspecified, status still 1). |

## Fixes applied in this batch

Bounded, confirmed-defect fixes only; no GNU 9.11 broadening. All were
verified fail-closed: each proving test fails against the pre-fix source and
passes after.

- `cmds/chmod/chmod.go` — octal modes are set absolutely when
  `POSIXLY_CORRECT` is present (the GNU keep-directory-setid rule remains the
  non-POSIX default); `X` evaluates the original unmodified mode only in
  POSIX mode and retains GNU 9.11's in-progress behavior otherwise; and
  `--reference` copies RFILE's mode exactly (`%05o`). Tests:
  `chmod_posix_test.go#TestModeApplyPOSIXOctalAbsolute`,
  `#TestChmodPOSIXModeOctalClearsDirectorySetID`,
  `#TestModeApplyXUsesOriginalUnmodifiedMode`,
  `#TestModeApplyXPreservesGNUBehaviorOutsidePOSIXMode`,
  `#TestChmodReferenceCopiesExactModeToDirectory`.
- `cmds/cp/cp.go` — the same-file and cannot-overwrite-directory diagnostics
  now precede the `-i` prompt (Issue 7 step 1 before step 3). Tests:
  `cp_posix_test.go#TestCpInteractiveSameFileDiagnosesWithoutPrompt`,
  `#TestCpInteractiveDirDestDiagnosesWithoutPrompt`,
  `#TestCpUpdateDoesNotBypassSameFileDiagnostic`.
- `cmds/cp/cp.go` + `owner_unix.go` + `owner_other.go` — `-p` duplicates
  ownership before mode and masks S_ISUID/S_ISGID out of the duplicated mode
  when uid/gid duplication fails (with a `chownFn` seam on unix; `!unix`
  reports non-duplicable). Tests:
  `cp_posix_unix_test.go#TestCpPreserveModeAppliedAfterOwnership`,
  `#TestCpPreserveClearsSetuidWhenOwnershipFails`.
- `cmds/ln/ln.go` — the implementation-defined default for a hard link to a
  symlink source is pinned `-P` on supported Unix syscall targets via
  `hardLinkPhysical` (linkat, no AT_SYMLINK_FOLLOW) instead of `os.Link`, whose
  semantics diverged between darwin (logical) and Linux (physical). Tests:
  `ln_test.go#TestLnDefaultHardLinkToSymlinkIsPhysical`,
  `#TestLnLastOfLogicalPhysicalWins`.
- `cmds/mkdir/mkdir.go` — `-p` operands are separator-trimmed
  (`trimTrailingSep`, volume-root-safe, also strips a trailing `/.`) so the
  final directory of `mkdir -p dir/` gets the `-m`/default mode instead of the
  intermediate mode. Tests:
  `mkdir_unix_test.go#TestMkdirParentsTrailingSlashFinalMode`,
  `mkdir_test.go#TestMkdirParentsTrailingSlash`.
- `cmds/mv/rename_unix.go` + `rename_other.go` (new) — `moverDeps.rename`
  uses direct `syscall.Rename` on unix so an existing empty destination
  directory is atomically replaced per the rename() equivalence clause
  (`os.Rename`'s stdlib pre-check returned EEXIST without calling rename(2));
  `!unix` keeps `os.Rename`. Test:
  `mv_test.go#TestMvDirectoryOntoExistingDirectoryInTarget`.
- `cmds/mv/mv.go` — same-file detection now compares final directory-entry
  identities with `Lstat`, so distinct symlinks to one referent can replace
  one another, and GNU `-u` cannot bypass a true same-entry diagnostic. Tests:
  `mv_test.go#TestMvDistinctSymlinksToSameReferentAreNotSameFile`,
  `#TestMvUpdateDoesNotBypassSameFileDiagnostic`.
- `cmds/rm/rm.go` — the identity-checked root refusal now also guards the
  `-d` removal path, and the implicit write-protection prompt skips symlink
  operands (unlink targets the link; `access(2)` consulted the referent and
  prompted for dangling links). Tests:
  `rm_test.go#TestRmPreserveRootGuardsDashDRemoval`,
  `#TestRmImplicitPromptSkipsSymlinkOperands`.

`chgrp`, `chown`, `mkfifo`, and `tail` needed no fixes — evidence-closure
tests only (option-terminator and `-` operand pins, invalid-group operand,
stdout-not-used assertions, omitted-who umask under `-`, process-umask default
lane, exact single-file stdout, and stdout write-failure status).

## Gate notes

The manifest routing-evidence lane resolves `bashy:` references against a
sibling `bashy` checkout next to this repository (as `sh:` references resolve
against the sibling `sh`). This Weave workspace initially lacked that sibling,
which made `scripts/posix_manifest.py --check` and 23 of the 40 unit tests
fail at HEAD for environmental reasons; the sibling was materialized by
cloning the umbrella's bashy repository, after which the full baseline was
green before any change in this batch. All 22 pre-existing
`shell_routing_evidence` references are preserved untouched.
After reconciliation with the five-command shell semantic batch, the visible
state assertion is 2 verified / 35 partial / 79 unverified. This batch itself
promotes exactly ten previously unverified Go-owned rows to partial.
