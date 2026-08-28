# Profile D `ls` Repair — 2026-08-28

Primary contract: [The Open Group POSIX.1 Issue 7, 2016 Edition `ls`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html).
GNU Coreutils 9.11 controls exit-status granularity for `ls`, which documents
its own `0`/`1`/`2` convention (0 ok, 1 minor problem, 2 serious trouble) rather
than the generic "2 = usage" rule; rule 3 (upstream semantics are immutable)
makes that `ls`-specific convention the target.

This pass re-audited the normalized Profile D `ls` residuals (11 FAIL, 1
UNRESOLVED) against the specification and separated genuine applet defects — now
fixed — from environment, mount, locale, and process-launcher residuals that are
not applet behavior. No harness workarounds were used.

## Applet defects fixed

Both defects were reproduced directly (`ls` on an unreadable directory) and are
covered by focused regression tests in `ls_posix_test.go`.

1. **Command-line directory access failures now exit `2`, traversal failures
   exit `1`.** Failing to open, read, or close a directory named as a
   command-line operand is "serious" (status 2); the identical failure on a
   directory reached during `-R` traversal is a "minor problem" (status 1). GNU
   makes exactly this `command_line_arg` distinction. Previously every
   directory-listing failure returned `1`, so an inaccessible command-line
   directory operand exited `1` while the sibling operand-access path in `run`
   already exited `2` — an internal inconsistency. A `commandLine` flag is now
   threaded through `listDir`/`listDirWithAncestors`, and `dirFailCode` selects
   the status. This resolves the inaccessible-root fixture blockers.
   Tests: `TestUnreadableCommandLineDirectoryExitsTwoWithoutHeader`,
   `TestUnreadableCommandLineDirectoryAmongOperands`,
   `TestUnreadableTraversedDirectoryExitsOne`.

2. **The `name:` header is printed only after a successful `opendir`/read.** The
   directory header (and its separating blank line) was written to stdout before
   the directory was opened, so an unopenable directory emitted a bogus
   `name:` header on stdout in addition to the stderr diagnostic. GNU emits the
   header only after the directory is successfully opened and read. The open and
   read now precede any stdout write, so a directory that cannot be listed
   produces only a stderr diagnostic and contributes nothing to stdout, while
   readable operands are still listed and separated correctly.
   Test coverage: the two multi-operand tests above assert the absent header.

## Honest residuals (not applet defects)

These normalized items are matched-control (the paired GNU run produced the same
environment-determined outcome) or belong to a layer outside the in-process
command. They are not converted into applet behavior changes.

* **Block-allocation fixture totals** and **directory access-time (`-u`)
  observations** are filesystem- and mount-dependent: `-s`/`total` report
  `st_blocks` allocation, and reading a directory perturbs its atime only under
  a mount's atime policy (`relatime`/`noatime`). On a matched mount the Bashy and
  GNU controls agree; the divergence requires a fixture whose allocation or
  atime policy differs measurably from the control, which is an integration
  boundary, not an applet defect.
* **Empty or looping symbolic links** were re-verified rather than changed: a
  command-line loop or dangling link under `-L` fails to access and exits `2`
  with a clear diagnostic; `-RL` ancestor-cycle detection reports the cycle and
  recovers to list siblings; without `-L` such links are listed literally. This
  matches GNU; no applet change is warranted.
* **Non-C `LC_COLLATE`, `LC_TIME`, and `LC_MESSAGES`** remain out of scope for
  the deterministic `LC_ALL=C` output contract, and an *unavailable* installed
  locale is matched-control (GNU also fails closed on an uninstalled locale).
* **Signal disposition and core-file behavior** belong to the process launcher
  and cannot be established by an in-process command test.

## The single unresolved residual

* **ACL / security-context marker (`+` after the mode string).** POSIX does not
  mandate the alternate-access-method marker, and displaying it faithfully
  requires platform ACL/xattr metadata support that is not yet wired into the
  applet's `sysInfo`. It is left explicitly UNRESOLVED rather than approximated:
  emitting or omitting `+` without real per-file ACL detection would violate the
  agent contract's "no silent approximation" rule.
