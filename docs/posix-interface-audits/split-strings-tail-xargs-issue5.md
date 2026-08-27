# `split`, `strings`, `tail`, and `xargs` Issue 5 POSIX.1 Issue 7 audit

Scope: Open Group POSIX.1-2008 Issue 7, 2016 Edition interfaces for exactly
`split`, `strings`, `tail`, and `xargs`, re-audited against repository
baseline `26d766f` (the Profile D applet gap audit's rankings). GNU behavior
is relevant only where Bashy deliberately retains an extension; it is not
conformance evidence. This re-audit follows the prior closure audits
[`env-split-tail-issue763.md`](env-split-tail-issue763.md),
[`cat-xargs-issue765.md`](cat-xargs-issue765.md), and
[`ln-mesg-strings-issue762.md`](ln-mesg-strings-issue762.md), which already
closed the bulk of the required option/operand/stream/status surface for
these four commands.

## Result

All required Issue 7 synopsis forms, options, option-arguments, operands,
stream semantics, and exit-status classes for all four commands were
re-verified present and correct by re-reading the normative clauses against
the current source and by re-running (and extending) the focused package
test suites. `strings` and `split` required no source change: the prior
closures already cover their POSIX-mandated surface, and the remaining
residuals (multibyte `{NAME_MAX}` accounting, translated diagnostics,
arbitrary installed-locale breadth) are truthful platform/locale integration
boundaries, not incorrect behavior.

Two concrete implementation gaps were found and fixed, both in the retained
GNU extension surface rather than in a POSIX-mandated clause, and both are
the kind of "known lower-risk I/O edge" the Profile D gap audit's residual
list names for these commands:

- **`tail --follow=name`** compared file identity across polls using an
  `inodeKey` helper that unconditionally returned `0` on every platform
  (`tail_unix.go` and `tail_windows.go`). Since the comparison
  `newIno != lastIno && lastIno != 0` can never be true when both sides are
  always `0`, a file replaced in place (the exact log-rotation shape:
  remove/rename the old path, create a new file at the same path) was never
  detected by identity at all. The only thing that ever caught a rotation
  was the independent `--max-unchanged-stats` no-size-change fallback — and
  only when the replacement file's size differed from the old file's for
  long enough, or never, if the new file happened to hold data of the same
  length. Fixed by comparing `os.FileInfo` values with the stdlib's
  cross-platform `os.SameFile` (already used elsewhere in this repository,
  e.g. `cmds/split`'s null-device check) instead of a hand-rolled,
  never-populated inode number. This also incidentally makes the identity
  check meaningful on Windows, where the old stub was equally inert.
  `TestTailFollowByNameDetectsSameSizeRotation` pins a same-byte-length
  atomic rename-over-path replacement being picked up immediately, not after
  waiting out the unchanged-stats counter.
- **`xargs -p`** unconditionally attempted `os.Open("/dev/tty")` for its
  yes/no prompt reader on every platform, including Windows, where that path
  does not exist. Rather than guess at an unverified Windows console-device
  substitute, this follows the repository's own established precedent for
  exactly this situation (`cmds/more`'s `tty_windows.go`, which explicitly
  refuses interactive terminal mode on Windows rather than approximate one):
  `xargs -p` now fails closed on Windows with a clear diagnostic naming the
  reason, instead of failing with a generic file-not-found message. Unix
  behavior (`/dev/tty`) is unchanged. `TestWindowsInteractiveModeIsExplicitly
  Unsupported` (type-checked by `crossvet`) pins the refusal.

## Verification

- `go test ./cmds/split/... ./cmds/strings/... ./cmds/tail/... ./cmds/xargs/...`
  and the same set with `-race`, default mode, `count=1` and `count=3`: all
  green.
- `go build ./...`; `GOOS={windows,linux,darwin} go vet` over the CI scope
  (excludes `external/`); the `js`/`wasip1` chmod/chgrp/chown wasm canaries;
  the `aix` `pkg/steward`/`pkg/policy/coord`/`cmds/dd` build canary; the
  `js`/`wasip1` `cmds/mv` build canaries. All pass, exercising the new
  `cmds/xargs/xargs_tty_{unix,windows}.go` and `cmds/tail/tail_{unix,windows}.go`
  changes on every target.
- `scripts/applet-test-coverage.sh` (PASS, 158 shipped packages) and
  `scripts/applet-matrix.py --check` (PASS, regenerated for the two new
  `cmds/xargs` files).
- `gofmt -l` clean on all changed files.

`scripts/crossvet.sh`'s bundled `python3 -m unittest scripts/posix_manifest_test.py`
and `posix_manifest.py --check` steps remain red in this workspace on a
**pre-existing, unrelated** condition identical before and after this audit's
changes (confirmed via `git stash`): `alias: shell routing evidence is
unavailable or unfocused`, the same adjacent-Bashy-checkout sibling-evidence
gap recorded in `env-split-tail-issue763.md`. It does not involve `split`,
`strings`, `tail`, or `xargs`. The cross-OS `go vet`/`go build` legs that
script would otherwise run were executed directly (see above) since the
early `set -e` failure prevents the script from reaching them.

## Residuals

Unchanged from the prior closure audits: translated (`LC_MESSAGES`/`NLSPATH`)
diagnostics remain absent across all four commands; `strings`'s locale
printability corpus is bounded (C/POSIX, UTF-8, and the carried single-byte
providers) rather than an exhaustive installed-locale database; `split`'s
multibyte-filename `{NAME_MAX}` accounting continues to rely on the
filesystem's own creation-time error rather than a portable pre-check,
because no single byte-count threshold is correct across every backing
filesystem; `xargs -p` real controlling-TTY integration is otherwise
unchanged; and `tail -f`'s multiple-operand-sequential-not-concurrent
extension deviation is unchanged. None of these are treated as certification
blockers under the Sprint 79 consolidated policy.
