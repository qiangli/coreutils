# `mkdir` POSIX.1-2008 Issue 7 Audit

Scope: `mkdir [-p] [-m mode] dir...` against The Open Group POSIX.1-2008
Issue 7, 2016 Edition utility page:
<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html>.

## 1. Options and Operands

The required `-m mode` and `-p` options are implemented. `dir` operands are
processed in argument order; an operand failure diagnoses that operand, records a
non-zero final status, and processing continues with later operands.

`-` is an ordinary pathname when protected from option parsing by `--`.
An empty pathname is rejected with a diagnostic. Without `-p`, any existing
directory or non-directory is an error. With `-p`, an existing directory,
including a symlink that resolves to a directory, is accepted without changing
the entry.

Evidence:
`TestMkdirSimple`, `TestMkdirExisting`, `TestMkdirContinuesAfterOperandError`,
`TestMkdirDashOperandIsPathname`, `TestMkdirEmptyOperandFailsAndContinues`,
`TestMkdirParentsAcceptsSymlinkToDirectory`.

## 2. Mode Grammar and Umask

`-m` accepts octal modes from `0` through `7777`, including set-user-ID,
set-group-ID, and sticky bits. Non-octal numeric strings are rejected instead
of being treated as octal. The GNU operator-numeric `+MODE` spelling remains a
documented retained extension outside the Issue 7 evidence surface; notably,
`-m +777` creates mode 0777 as GNU Coreutils 9.11 does.

Symbolic modes use the chmod-style grammar already carried by mkdir: `who`
lists, `+`, `-`, and `=` actions, `rwxXst` permissions, and single `u/g/o`
permission copies. Symbolic evaluation starts from the assumed directory
creation mode `a=rwx`; clauses with omitted `who` are filtered through the
effective umask.

For standalone Unix invocations the process umask remains authoritative. For
embedded Unix invocations, `RunContext.UmaskSet` supplies the virtual shell
umask without mutating the process-global umask. The virtual-mask-derived mode
is supplied to `mkdir` itself, so a permissive host-process umask cannot expose
the new directory more broadly before correction. A post-create check restores
bits removed by a stricter host-process umask while preserving special bits the
filesystem inherited from the parent when no explicit `-m` controls the mode.
This applies both to the default final directory mode and to implicit-who
symbolic modes.

Windows has no POSIX mode bits. `-m` therefore remains a loud unsupported
error, while an otherwise ordinary creation uses Windows ACL behavior and does
not approximate `RunContext.Umask` through `os.Chmod`'s unrelated read-only
attribute mapping.

Evidence:
`TestMkdirMode`, `TestMkdirModeErrors`, `TestMkdirSymbolicMode`,
`TestMkdirSymbolicModeStartsAtDefault`,
`TestMkdirSymbolicModeSubtractsFromDefault`, `TestMkdirSymbolicModeApply`,
`TestMkdirLeadingPlusNumericModeExtension`,
`TestMkdirVirtualUmaskDefaultAndParents`,
`TestMkdirVirtualUmaskSymbolicImplicitWho`,
`TestMkdirVirtualUmaskRestrictsInitialMkdirModes`,
`TestMkdirVirtualUmaskCorrectionPreservesInheritedSpecialBits`,
`TestMkdirVirtualUmaskPreservesInheritedSetgid`,
`TestMkdirVirtualUmaskDoesNotApproximatePOSIXModesOnWindows`.

## 3. Parent Creation

With `-p`, missing ancestors are created from left to right. Each created
ancestor receives `(0777 & ~umask) | 0300`, preserving owner write and search so
the walk can continue under restrictive umasks. That restricted mode is passed
to the initial `mkdir`, and any corrective chmod preserves inherited special
bits. Existing ancestors are left unchanged. `-m` applies only to a newly
created final directory.

Trailing slash and trailing `/.` operands are normalized for mkdir's creation
walk so that the final component remains final; it does not receive the
intermediate owner-write/search augmentation.

Evidence:
`TestMkdirParents`, `TestMkdirParentsTrailingSlash`,
`TestMkdirParentsRetainOwnerWriteAndSearch`,
`TestMkdirParentsTrailingSlashFinalMode`,
`TestMkdirVirtualUmaskRestrictsInitialMkdirModes`,
`TestMkdirVirtualUmaskPreservesInheritedSetgid`.

## 4. Error Handling and Seams

The implementation now has mkdir-local seams for `stat`, `mkdir`, and `chmod`.
Tests inject create and chmod failures without changing shared runtime
interfaces or other commands. Real temp-filesystem tests cover normal creation,
trailing slash behavior, symlink behavior, and permission-denied behavior on
Unix.

Injected and kernel errors are diagnosed on standard error and produce final
status `1`; usage and invalid-mode diagnostics produce status `2`.

Evidence:
`TestMkdirInjectedFilesystemErrors`,
`TestMkdirInjectedChmodErrorLeavesCreatedDirectoryAndFails`,
`TestMkdirParentsRejectsDanglingSymlink`,
`TestMkdirPermissionDeniedContinuesAfterOperand`.

## 5. Residuals

Diagnostics are fixed English strings; `LC_MESSAGES` and `NLSPATH` catalogs are
not implemented. Windows refuses `-m` and does not apply a virtual POSIX umask
because POSIX mode bits are not available there; default directory access is
ACL-owned. Filesystem inheritance of special directory bits is platform-owned;
the focused Unix test first proves setgid inheritance is available and skips if
it is not. The GNU `-v` extension remains supported outside the Issue 7
surface. `-Z/--context` is a pre-existing deterministic no-op rather than a
real SELinux implementation; it is not POSIX evidence and remains an explicit
GNU-fidelity residual.

## 6. Gate Record

Required local gate for this issue, run on 2026-08-25:

```sh
POSIXLY_CORRECT=1 go test -count=20 ./cmds/mkdir
POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/mkdir
POSIXLY_CORRECT=1 go vet ./cmds/mkdir
go test -count=20 ./cmds/mkdir
go test -race -count=5 ./cmds/mkdir
go test -shuffle=on -count=50 ./cmds/mkdir
go vet ./cmds/mkdir
GOOS=linux go vet ./cmds/mkdir
GOOS=darwin go vet ./cmds/mkdir
GOOS=windows go vet ./cmds/mkdir
GOOS=freebsd go vet ./cmds/mkdir
GOOS=aix GOARCH=ppc64 go build ./cmds/mkdir
./scripts/fmtcheck.sh
```

Results:

* `POSIXLY_CORRECT=1 go test -count=20 ./cmds/mkdir` passed.
* `POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/mkdir` passed.
* `POSIXLY_CORRECT=1 go vet ./cmds/mkdir` passed.
* Default tests (20 runs), race tests (5 runs), and shuffled tests (50 runs)
  passed.
* Native and Linux/Darwin/Windows/FreeBSD vet passed; the Windows test binary
  cross-compiled successfully.
* `GOOS=aix GOARCH=ppc64 go build ./cmds/mkdir` passed.
* `./scripts/fmtcheck.sh` passed.

Default regression:

* Repository-wide `go test` excluding `external/` passed `cmds/mkdir` and the
  rest of the command surface, but failed in unrelated
  `cmds/batch#TestBatchDiagnosticUsesInvocationTZAndLCTIME` (the known
  localized weekday expectation) and
  `pkg/weave#TestYcodeDeclaredProbeUsesResolvedPath` (an unrelated resolved
  executable probe). `scripts/crossvet.sh` stops before its target builds on
  the main-branch `applet matrix was stale` validation failure; the affected
  target builds/vets above were run directly.
