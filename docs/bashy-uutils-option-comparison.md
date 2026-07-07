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
