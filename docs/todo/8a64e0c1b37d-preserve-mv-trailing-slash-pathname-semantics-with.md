---
id: 8a64e0c1b37d
kind: task
title: Preserve mv trailing-slash pathname semantics without breaking -T
seq: 3
status: todo
priority: p2
created: 2026-08-11T19:00:00Z
sprint: 100
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
