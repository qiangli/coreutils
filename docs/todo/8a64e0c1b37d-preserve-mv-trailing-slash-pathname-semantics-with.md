---
id: 8a64e0c1b37d
kind: task
title: Preserve mv trailing-slash pathname semantics without breaking -T
seq: 3
status: done
priority: p2
created: 2026-08-11T19:00:00Z
assignee: claude-opus5.5
sprint: 110
sprint_id: 26c12bac-7289-517f-ad46-01e2c117cad7
sprint_title: Revalidate Go 1.27 GNU and POSIX conformance
closed: 2026-09-30T11:22:06.917244Z
closed_by: claude-opus5.5
---

Fix the `mv` raw-operand/path-normalization gap described in
`docs/sprint-49-salvage-reconciliation.md`. A destination ending in `/` must
resolve to a directory before the normalized `RunContext.Path` value erases
that syntax, but this validation must remain separate from GNU `-T`
(`--no-target-directory`). Report a missing component distinctly from an
existing non-directory, preserve every source on failure, and retain
multi-source and `--strip-trailing-slashes` behavior.

Required tests: existing regular file, nonexistent path, real directory,
symlink to directory, `-T` with a directory operand, multiple sources, and the
explicit stripping extension. Prove the behavioral tests red on the current
base, then run focused normal/race tests, matrix regeneration/check, and full
crossvet. Do not infer a TP identity or retirement from numeric ordering.

## 2026-09-01 — this is now CI-blocking, and it is LINUX-ONLY

`cmds/mv` fails the `test` workflow on ubuntu-latest (GitHub Actions runs
33513814323, 33515325794):

    --- FAIL: TestMvNoTargetDirectoryTrailingSlashOnExistingDir
        mv_test.go:453: existing directory misdiagnosed as not-a-directory:
        "mv: cannot move 'src' to 'somedir/': Not a directory"

It PASSES on darwin, so a macOS-only check will report this story as done when
it is not. Verify on Linux.

CORRECTION (same day): an earlier revision of this note also claimed
`TestMvCopyFallbackFailures` fails on linux. It does not. That was observed in
a golang:1.26 container running as ROOT, where the `chmod 0o000` the test uses
to make the copy fail is ignored; on the GitHub runner, an ordinary user, it
passes. It is not part of this story.

These tests were written red on purpose per the contract above, so nothing here
is a new regression. They were simply invisible until 2026-09-01: coreutils CI
had been failing at the BUILD step for weeks (`pkg/webconsole` gained a
`../filebrowser` sibling replace the workflow never cloned), so no coreutils
test ran in CI at all. Fixed in c3663683.

## POSIX-cert impact

`mv` is a POSIX-required utility in the certification scope
(docs/posix-required-commands.md), and a trailing slash is not cosmetic
syntax: POSIX pathname resolution requires a pathname ending in `/` to resolve
to a directory. Reporting an EXISTING directory as "Not a directory" is a
conformance defect in a certified utility, not only a CI failure, so this
story is tracked under the POSIX cert sprint.

The `-T` half stays a GNU extension and must not be conflated with it — the
contract above already separates them, and that separation is the reason this
is delicate rather than a one-line fix.

## Review 2026-09-30 (steward)

- Status: still reproduces - `linux ... cmds/mv TestMvNoTargetDirectoryTrailingSlashOnExistingDir` is still baselined in `test/known-failures.txt` at coreutils dc805500. 091f82aa (2026-09-22, operand rebuilt in the caller's spelling) touched mv's destination arithmetic but did not remove the baseline entry.
- Outdated: CI run IDs and "golang:1.26" context only; `docs/sprint-49-salvage-reconciliation.md` still exists. Destination arithmetic now goes through the `tool.OperandJoin/OperandDir/OperandClean` helpers (091f82aa).
- Next step: actionable today, independent of the licensed host. Capture the raw trailing-slash operand before `RunContext.Path` normalizes it (use the new operand helpers), validate "must be a directory" separately from `-T`, write the listed tests red first, fix, delete the known-failures line.
- Acceptance: all required tests green on Linux CI and darwin; known-failures line removed; mv:* identities no worse in the fresh baseline arm.
- Depends on: nothing.

## Conductor brief 2026-09-30 (Sprint #110 heat)

Toolchain: go1.27.1. You may be on darwin: the failure is LINUX-ONLY ({PATH_MAX} 4096 vs 1024), so do not
declare victory from a darwin run. If podman works in your environment, the Linux check is
  /Users/qiangli/.bashy/sprint/110/gate/linux-go-test.sh -count=1 <pkg> [-run X]
(golang:1.27, ordinary user, pinned ../sh). Otherwise reason from the Linux regime and make the
regression reproducible on darwin too where you can (a package-level seam, never a sleep).
Scope: only the files named below plus test/known-failures.txt (delete the story's baseline line in the SAME commit).
Commit with trailers Sprint: #110 / Story: #<seq> / Story-ID: <id> from this file's front matter.

Gate (graded outside your booth, both must pass):
  linux-go-test.sh -count=1 ./cmds/mv ; darwin go test -count=1 ./cmds/mv ; the
  'cmds/mv TestMvNoTargetDirectoryTrailingSlashOnExistingDir' line gone from test/known-failures.txt ;
  every package still compiles (go test -run '^$' ./...).
New tests in cmds/mv/*_test.go are expected (the seven cases listed above); never weaken or delete existing tests.
Unverified hypothesis: the Linux path treats "dir/" differently from darwin at a stat/rename call -- confirm it first, then validate the raw
trailing-slash operand yourself (tool.OperandJoin/OperandDir/OperandClean) before normalization, and keep -T separate.
Scope: cmds/mv/** (and pkg/tool operand helpers only if strictly needed).
