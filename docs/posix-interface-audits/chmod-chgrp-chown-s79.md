# `chmod`, `chgrp`, and `chown` — POSIX Issue 7 interface closure

Scope: `cmds/chmod`, `cmds/chgrp`, `cmds/chown`, and their shared ownership
walker/option parser. Normative authority is The Open Group Base
Specifications Issue 7, IEEE Std 1003.1-2008, 2016 Edition:

- [`chmod`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html)
- [`chgrp`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html)
- [`chown`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html)

GNU behavior is not conformance authority. Existing long options are retained
as compatibility extensions, but neither their presence nor their output is
used below as POSIX evidence. The applet matrix is regenerated when named test
products change; the shared POSIX interface ledger remains separately owned.

## Result

The three required interfaces are closed for the repository's supported Unix
implementation and fail closed on targets without POSIX ownership/mode
semantics. This run corrected seven implementation defects:

1. `chmod` previously selected absolute octal modes and the Issue 7 meaning of
   `X` only when `POSIXLY_CORRECT` was present. It now uses the required mode
   language on every invocation: octal is absolute and `X` examines the file's
   unmodified mode.
2. Embedded `chmod` previously ignored `RunContext.Umask` and inspected the
   process-global mask. Omitted-`who` symbolic actions now use the invoking
   shell's virtual mask; standalone use snapshots the inherited process mask
   under a mutex.
3. `chgrp` previously issued `chown(path, -1, gid)`. It now reads the selected
   file's owner and passes that UID with the requested GID, exactly matching
   the action specified by the utility page. Ownership metadata that cannot be
   represented is an error, not a guessed transition.
4. `chmod -R` did not reach the whole hierarchy when the mode removed
   permissions. It changed each directory *before* listing it, so
   `chmod -R 000 dir` set the directory to `0000` and was then unable to read
   it: everything below kept its old mode and the command exited 1. Issue 7
   requires `-R` to change "the directory and all files in the file hierarchy
   below it", so this was a failure to perform the required operation, not a
   reporting defect. `chmod` now uses the same shared hierarchy walker as the
   ownership commands, which hands a directory to the command only after its
   entries; the case above now changes every file and exits 0. Adopting the
   shared walker also gives `chmod` the cycle and unreadable-directory
   diagnostics it previously lacked — a directory cycle used to stop the walk
   silently.
5. `chmod` skipped the native operation when the computed mode equaled the
   observed mode. That bypassed permission checks, ctime, and filesystem side
   effects and could report false success. Every selected file now reaches the
   transition provider; equality affects only `-c`/`-v` wording.
6. Numeric owner/group parsing accepted every non-sentinel `uint32` and then
   converted it to Go `int`. Values above `MaxInt32` became negative on
   Linux/386 and collided with internal unchanged sentinels. Operand, observed,
   and reference IDs now fail closed unless the host `int` can represent them.
7. `chmod`'s dash-mode pre-scan could steal a separately-spelled
   `--reference` value such as `-w`, and its order scanner ignored an
   unambiguous `--no-deref` abbreviation. The scan now protects option values
   and applies the same long-option abbreviation rule as the framework parser.

The ownership transition functions remain injectable. Real temporary-file
products exercise transitions safe for an unprivileged caller; injected
providers cover `EPERM` and read-only-filesystem failures without changing a
foreign owner, requiring root, or mounting unsafe filesystems.

## Required synopses and options

| Utility | Required synopsis | Required options | Disposition |
| --- | --- | --- | --- |
| `chmod` | `chmod [-R] mode file...` | `-R` | `-R` changes each directory operand and the hierarchy below it. Zero files, a missing mode, an invalid mode, and an unknown option diagnose and return nonzero. |
| `chgrp` | `chgrp [-h] group file...`; `chgrp -R [-H|-L|-P] group file...` | `-h`, `-H`, `-L`, `-P`, `-R` | Both forms are accepted. `-H/-L/-P` affect recursive traversal only; the last one specified wins. |
| `chown` | `chown [-h] owner[:group] file...`; `chown -R [-H|-L|-P] owner[:group] file...` | `-h`, `-H`, `-L`, `-P`, `-R` | Both forms are accepted with the same traversal rules as `chgrp`. A missing group portion leaves group ownership unchanged. |

`--` ends option parsing, so a subsequent `-`, `-H`, or similar spelling is a
pathname/operand. A lone `-` is never standard input. Usage failures return 2,
which is within POSIX's required `>0` class; operational failures return 1.

The retained `-c`, `-f`, `-v`, `--reference`, `--from`, root guards, and long
dereference spellings are extensions. The ownership commands also accept
empty-owner/trailing-colon GNU forms as extensions. Required behavior does not
depend on them. `-h` combined with `-R` is accepted as a defined extension;
the two POSIX synopsis forms themselves are independently covered.

## Recursive ownership traversal and link target matrix

Traversal and the target of the ownership syscall are separate decisions.
`cmds/internal/hierwalk` decides which hierarchy entries are reached;
`-h`/the selected recursive mode decides whether a reached symbolic link is
passed to a follow or no-follow ownership operation. All three utilities now
share that walker, so recursion, cycle handling, unreadable-directory
reporting, and the root guard are one implementation rather than three.

| Invocation | Operand link to directory | Link encountered below operand | Ownership operation on a reached link |
| --- | --- | --- | --- |
| no `-R`, no `-h` | no hierarchy walk | n/a | referent (`chown`) |
| no `-R`, `-h` | no hierarchy walk | n/a | link (`lchown` equivalent) |
| `-R -P` | not followed | not followed | link; no referent subtree |
| `-R -H` | followed when it names a directory | not traversed | referent by the normal chown-equivalent action; no nested referent descent |
| `-R -L` | followed | followed when it names a directory | referent |

POSIX leaves the recursive default unspecified when none of `-H/-L/-P` is
present. This implementation deliberately chooses physical `-P`, the safest
fail-closed choice. Repeated and clustered selectors preserve argument order;
the last selector wins. `-H` follows a whole command-line link chain, but does
not turn links discovered later into traversal roots. `-L` detects ancestor
identity, visits a repeated link entry once, and stops descending rather than
looping. A corrupt physical directory cycle is diagnosed, gives nonzero
status, and does not stop sibling processing.

Focused evidence:

- `cmds/chown/hierarchy_unix_test.go`: `TestChownTraversalModes`,
  `TestChownTraversalAndDereferenceAreOrthogonal`,
  `TestChownCommandLineLinkChainIsFollowed`, dangling-link and cycle tests.
- `cmds/chgrp/hierarchy_unix_test.go`: the corresponding `TestChgrp...`
  products, including both selector orders and command-line/nested links.
- `cmds/internal/hierwalk/hierwalk_test.go` and
  `cmds/internal/linkopts/linkopts_test.go`: isolated traversal, chain, loop,
  clustering, `--`, and option-value products.

### Walk order is what makes `-R` reachable

A directory is handed to the command only after its entries. For the ownership
commands that avoids giving away a directory the walk still has to read; for
`chmod` it is what allows the required operation to complete at all. A mode
that removes search permission from a directory would otherwise put that
directory's own contents beyond the reach of the command that was told to
change them. `chmod -R 000 dir` is the observable case: children first, it
changes every file and exits 0; directory first, everything below keeps its old
mode. The reverse case is bounded by the filesystem rather than by the order —
a directory that is already unreadable is diagnosed, is still changed itself,
and its unreachable contents are reported as a nonzero status.

Evidence: `cmds/chmod/recursion_unix_test.go` —
`TestChmodRecursiveRemovingPermissionsReachesWholeHierarchy` (the requirement,
as a real filesystem product owned by the caller),
`TestChmodRecursiveVisitsChildrenBeforeDirectory` (the same ordering read off
`-v` output), `TestChmodRecursiveUnreadableDirectoryIsDiagnosedAndStillChanged`
(diagnose, still change, continue to the next operand), and
`TestChmodRecursiveSymlinkLoopTerminates`.

The Issue 7 `chmod` page defines only `-R` and does not select `-H/-L/-P` or a
recursive symbolic-link traversal policy. The implementation's physical
recursive default (do not follow or modify link entries) and its explicit
link options are documented extensions, not claimed as required behavior.
Without `-R`, a normal file operand is resolved as a pathname and a final
symbolic link therefore names its referent.

## Ownership operand grammar and providers

`GROUP`, `owner`, and the optional `group` half are resolved in this order:

1. Query the relevant user/group database using the exact operand spelling.
2. If the database returns a matching name, use that record's numeric ID. A
   name consisting entirely of digits therefore wins over the same spelling
   as a numeric ID, as Issue 7 explicitly requires.
3. Only an actual unknown-name result permits numeric interpretation.
   Operational identity-provider errors are diagnosed and fail closed.
4. A numeric spelling is one or more ASCII decimal digits. Signs, mixed text,
   `uint32` overflow, the all-ones `chown(2)` “unchanged” sentinel, and values
   not representable by the host Go `int` are rejected.

For required `chown owner` the group argument is `-1` and is unchanged. For
`chown owner:group`, both resolved IDs are passed in one transition. `chgrp`
stats using the same follow/no-follow policy as the ensuing operation and
passes `(observed_uid, requested_gid)`. That preserves the exact permission
checks, atomic owner/group product, set-ID consequences, and ctime behavior of
the required chown-equivalent operation.

Focused evidence includes `TestChownNameIsPreferredOverNumber`,
`TestChgrpNameIsPreferredOverNumber`, reference-ID non-reinterpretation tests,
`TestChownNumericGrammarAndLookupFailures`,
`TestChgrpNumericGrammarAndLookupFailures`, and
`TestChgrpObservedOwnerProvider`. The retained `chgrp --from` extension uses
the same resolution and width rules, covered directly and through the command
by `TestChgrpFromNumericGrammarAndLookupFailures` and
`TestChgrpFromRuntimeRejectsInvalidOrUnavailableOwner`. These are named
top-level products rather than being hidden beneath an unrelated
cycle-diagnostic test.

## `chmod` mode language

| Requirement | Implementation/evidence |
| --- | --- |
| Non-negative octal integer | Only octal digits are accepted, values above `07777` are rejected, and all listed bits are set absolutely. `TestModeApplyPOSIXOctalAbsolute` and `TestChmodPOSIXModeOctalClearsDirectorySetID`. |
| Ordered clauses/actions | Comma-separated clauses and multiple actions under one `who` are parsed and applied left-to-right. Bare `+`/`-` do nothing; bare `=` clears the selected classes. |
| `who` | `u`, `g`, `o`, repeated combinations, and `a == ugo`; omitted `who` selects all classes subject to the invocation umask. |
| `perm` | `r`, `w`, `x`, `X`, `s`, and XSI `t`; `o+s` is accepted and does not modify either set-ID bit. |
| `permcopy` | A single `u`, `g`, or `o` copies the current permission triad and can be followed by another action, including `g=o-w`. |
| `X` | Uses the original, unmodified mode for the whole invocation, or always supplies execute/search for a directory. `TestModeApplyXUsesOriginalUnmodifiedMode`. |
| Omitted-`who` umask | `+`, `-`, and the set phase of `=` leave corresponding masked permissions untouched; bare omitted-`who` `=` first clears every POSIX mode bit. `TestChmodSymbolicModeUsesInvocationUmask` is end-to-end virtual-umask evidence. |
| Set-ID/sticky consequences | Requests are passed to the kernel. The standard's implementation-defined non-regular-file latitude is not replaced with an approximation. Regular-file set/clear and directory absolute clear are real filesystem products. |

A mode operand beginning with `-` is rescued only when its body is entirely
mode grammar characters; `-- -w file` is also accepted. This prevents the
required `chmod -- -w file` form from being mistaken for options. The scanner
does not reinterpret values consumed by `--reference`; focused extension
products cover both `--reference -w target` and abbreviated `--no-deref`.

## Filesystem and metadata effects

- Every selected mode operation, including an equal-mode request, uses the
  native `chmod` operation. `TestChmodSameModeFailuresContinue` injects EPERM
  and EROFS on equal-mode files and proves that both failures are reported and
  a later equal-mode operand is still attempted. A hard-link product proves
  that a successful operation updates the inode reached through either name,
  including permissions, set-ID bits, and ctime.
- `chown` and `chgrp` issue the ownership operation even when the requested IDs
  already match. This is necessary because a successful unprivileged
  ownership call clears set-user-ID/set-group-ID on regular files and updates
  ctime. Existing real products prove set-ID clearing; this run adds native
  ctime products. Those products wait until the filesystem demonstrably
  exposes a timestamp later than the fixture's baseline; they do not assume a
  fixed 2 ms timestamp resolution.
- Default ownership operations on a symbolic-link operand update the
  referent's ctime. `-h` updates the link's ctime while leaving the referent's
  ctime unchanged. The same fixtures include hard-link aliases and verify
  inode metadata identity.
- No utility reads standard input or creates a standard output file. Required
  invocations write no standard output. Diagnostics use standard error only.

Real products are confined to caller-owned files, directories, hard links,
and symbolic links beneath `t.TempDir()`. No test changes a foreign UID/GID,
uses a host command, mounts a filesystem, or requires privilege. Filesystems
that truthfully cannot retain a requested set-ID bit cause that optional
platform product to skip rather than fabricate success.

## Errors, continuation, and status

Every named file operand is independent. Missing entries, failed stats,
unreadable directories, dangling referents, permission denial, read-only
filesystem errors, cycle detection, and transition failures diagnose the
display pathname as supplied by the caller, set final status nonzero, and do
not prevent later operands/siblings from being attempted.

The production bindings use only Go/native syscalls (`os.Stat`, `os.Lstat`,
`os.ReadDir`, `os.Chmod`, `os.Chown`, and `os.Lchown`); no host command is
executed. The `changeMode`, `changeOwner`, and `changeGroup` providers allow
otherwise unsafe transition failures to be injected.
`TestChmodSameModeFailuresContinue`, `TestChgrpTransitionFailuresContinue`, and
`TestChownTransitionFailuresContinue` inject permission and read-only errors
and prove the later operand is still reached. Real unreadable-directory and
missing-operand products cover filesystem traversal failures. Silent
compatibility options suppress their documented diagnostics but never turn a
failed operation into status zero.

## Platform disposition

Unix builds use native mode and ownership metadata. The non-Unix builds
(`chmod_other.go`, `chgrp_other.go`, `chown_other.go`, each `//go:build !unix`)
return status 1 with a not-supported diagnostic naming the platform;
they do not map POSIX IDs or mode bits onto unrelated host attributes. Unix
paths also fail closed if `FileInfo.Sys()` cannot supply `syscall.Stat_t`.
The acceptance gate below covers Linux, Darwin, Windows, WebAssembly, and AIX
canaries. Linux/386 also compiles the focused test binaries and executes the
numeric grammar products under `qemu-i386-static`.

## Focused gate

The acceptance sequence for this issue is:

```text
gofmt -w <changed Go files>
go test -count=20 ./cmds/chmod ./cmds/chgrp ./cmds/chown ./cmds/internal/hierwalk ./cmds/internal/linkopts
go test -race -count=5 ./cmds/chmod ./cmds/chgrp ./cmds/chown ./cmds/internal/hierwalk ./cmds/internal/linkopts
go vet ./cmds/chmod ./cmds/chgrp ./cmds/chown ./cmds/internal/hierwalk ./cmds/internal/linkopts
scripts/crossvet.sh
```

Observed results are recorded in the issue handoff after the sequence runs; an
exit code alone is not treated as proof, so package output and the post-gate
worktree are checked.
