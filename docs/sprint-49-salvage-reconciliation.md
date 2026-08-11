# Sprint 49A salvage reconciliation

This note records the durable conclusions recovered from the temporary
`docs/s49a-salvage/` bundle in the dhnt umbrella. It is planning input, not
VSC-PCTS evidence, and it retires no blocker identity.

Reconciled against coreutils `4a834b862444107d0a30c6102e5cd81ff1c73723`
on 2026-08-11.

## Actionable source gaps

### `chown` and `chgrp`

The current recursive implementation still uses `filepath.WalkDir`, which
does not descend into symlinked directories. Consequently `-R -L` cannot
perform a logical traversal. The current option representation also collapses
`-H`, `-L`, `-P`, `--dereference`, and `-h` into booleans before execution,
so it cannot reliably preserve required last-option behavior or keep traversal
policy separate from whether the symlink or referent is changed.

The repair must use a cycle-safe walker, model traversal and affect modes
separately, and cover live directory links, dangling links, loops, and both
orders of every conflicting option. See todo `31d7c9a82f44`.

### `mv`

The current path normalization loses evidence that the destination operand
ended in `/`. A previous candidate correctly stopped a regular-file
destination from being treated as a directory, but incorrectly conflated
pathname resolution with GNU `-T` (`--no-target-directory`) and reported a
real directory as `Not a directory`.

The repair must preserve the raw trailing-slash condition through parsing,
distinguish ENOENT from ENOTDIR, and let `-T` reach its ordinary destination
semantics after pathname validation. See todo `8a64e0c1b37d`.

## Revalidated without a new source patch

### `find`

The proposed recovered tests covered `-exec`, time/link primaries, symbolic
`-perm`, `-xdev`, `-depth`, and `-L`. Current package tests already cover
those seams more comprehensively in `find_test.go`, `conformance_test.go`, and
`exec*_test.go`; `go test -count=1 ./cmds/find` passes at the reconciled head.
The recovered `-exec /usr/bin/true` test was not imported because its absolute
host command is not cross-platform or hermetic. No public blocker identity was
mapped, so this establishes no retirement; an affected native comparison is
still required for certification credit.

### `basename`, `id`, `ln`, `pathchk`, and `uname`

The recovered triage found no public red-on-base behavior. Several suspected
rows were unspecified behavior or GNU extensions rather than POSIX defects.
No source change or affected-arm claim follows from that triage.

## Rejected evidence packets

The recovered `od`/`tail` packet mixed POSIX requirements with GNU-specific or
target-model presentation assumptions (offset widths, default block layout,
and host integer/float representation). Its tests may be useful as explicitly
labelled GNU/target regressions, but they are not POSIX evidence as written.

The recovered `chown`/`chgrp` and `mv` reviews remain useful only for the
counterexamples summarized above. Their workspace paths, superseded commit
SHAs, and candidate-specific verdicts are intentionally not retained here.

## Certification boundary

Nothing in this reconciliation changes the campaign count. A source fix must
pass package tests, focused race tests, the generated applet matrix, and
`scripts/crossvet.sh`; retirement additionally requires a current pinned
Bashy/GNU affected-target comparison with zero caps, missing outcomes, or
manual resolutions.
