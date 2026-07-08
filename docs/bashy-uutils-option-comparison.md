# bashy vs uutils coreutils option comparison

Generated on 2026-07-07 from live command output after rebuilding
`./cmd/coreutils` to `/private/tmp/coreutils`.

- Reference: `reference/uutils-coreutils/target/release/coreutils`
- Local binary: `/private/tmp/coreutils`
- Comparison scope: command inventory and declared option tokens in `--help`
- Bash builtin exclusions: `[`, `kill`, `printf`, `test`

This is a declared CLI-surface comparison, not a full behavioral conformance
test. Matching option spelling does not by itself prove identical semantics.

## Summary

| Measure | Result |
|---|---:|
| uutils commands missing locally, excluding bash builtins | 0 |
| uutils commands intentionally covered by bash builtins | 4 |
| Commands with non-builtin option-token discrepancies | 0 |

## Command Coverage

| Category | Commands |
|---|---|
| Missing native commands ignored because bash provides them | `[`, `kill`, `printf`, `test` |
| Missing non-builtin commands | none |

## Option Parity

The artifact-aware option extraction compares only declared option tokens from
option-definition lines and ignores prose/table artifacts such as `.env-style`
and descriptive text in `pr --help`.

| Command scope | Missing uutils options locally |
|---|---|
| All overlapping non-builtin commands | none |

## Notes

- `uutils-coreutils` is MIT licensed and may be used as a reference for future
  ports; substantial adaptations should be attributed in the relevant source
  comments and license notes.
- The four ignored commands are bash builtins in the importing `bashy` surface,
  so native Go command parity intentionally excludes them.

## Method blind spots + deep-parity addendum (2026-07-08)

The visible-help-token comparison above cannot see three classes of
difference. A source-level re-validation (uutils clap definitions parsed
from `reference/uutils-coreutils` Rust source, functionally probed against
the local binary) found and a follow-up sprint closed them:

1. **Hidden-but-accepted flags** (present in uutils/GNU parsers, absent
   from `--help` on both sides). Closed: `uname --sysname/--release`
   (obsolescent, hidden), `date --rfc-822/--rfc-2822/--uct` (deprecated
   GNU spellings, hidden), `cksum -b/-t` (deprecated, hidden), `pr -f`
   (visible; equals `-F`), `uname -p/-i` (visible; GNU-documented).
2. **Value words, not option tokens**: `du --time=atime|access|use|
   ctime|status` (GNU set — `birth` is uutils-only and stays rejected)
   and `ls --time=…` incl. `birth/creation` (GNU 9+). Closed.
3. **Long-option abbreviation**: uutils sets clap `infer_long_args`,
   matching GNU getopt_long unambiguous-prefix matching. Now implemented
   framework-wide in `tool.Parse` (exact match wins; ambiguous prefixes
   error `option '--x' is ambiguous; possibilities: …`, exit 2), so
   e.g. `cp --parent` and `wc --line` parse as in GNU/uutils.

4. **chcon** (closed 2026-07-08, follow-up round): full GNU option
   surface — `-R` with `-H/-L/-P` traversal, `--dereference`/`-h`,
   `-u/-r/-t/-l` component mode, `--reference`, `-v`,
   `--preserve-root` — parsing and usage errors identical on every
   platform, relabeling Linux-only as before. NOTE: the macOS-built
   uutils binary used for the table above omits `chcon`/`runcon`
   entirely (feature-gated), so this row was invisible to the
   help-token method; the source-level extractor now reports zero
   residual chcon gaps.

Known remaining delta, by policy:

- **uutils-only inventions are deliberately excluded** (`--l` on ls,
  `---presume-input-pipe`/`---io-blksize`/`dis` internal test hooks,
  `du --time=birth`): conformance is judged against GNU documentation;
  uutils extensions are not upstream semantics.
