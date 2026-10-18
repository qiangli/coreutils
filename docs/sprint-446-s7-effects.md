# S446 S7: conditional local effect preview

Story #2124 / 4ad82fdaf71e, based on coreutils 8c2706f2.

`rm`, `mv`, `cp`, `ln`, `chmod`, `chown`, `touch`, `mkdir`, `rmdir`,
`truncate`, and `install` declare `Tool.PreviewLevel = "local-preview"`.
Outside `VSC_PROFILE=cert` they accept long-only `--dry-run` (and the common
separator alias `--dry_run`). Put options before operands for POSIX applets
that stop parsing at the first operand. `--dry-run=false` executes normally.
Other commands do not acquire this flag; native command flags still win.

`rc.Effect(op, path)` returns true on the normal path, without recording or
writing anything. Under preview it appends a `PlannedEffect` to `rc.Effects`,
writes a quoted conditional plan to stdout, and returns false. Each caller
skips its entire mutation block on false. The registered invocation wrapper
clears preview state before each run and reports plan-output failure as a
nonzero exit, without allowing mutation. Cert does not register the flag,
and the effect seam is inert even if internal preview state was set.

The operation blocks cover file creation/open-for-write, replacement/backups,
recursive removal/copy/move, parent creation, metadata/ACL updates, timestamps,
and cross-device move fallback. Preview performs local metadata queries and
may traverse directories for chmod/chown. It does not open file contents,
consume confirmations, or run external programs. As with other read-only
filesystem queries, the OS may update access metadata during directory reads.
On non-Unix platforms chown retains its existing no-op contract and records
no ownership mutation.

## KISS boundary and acceptance limits

A plan is conditional, not evidence that execution would succeed. Normal
argument validation and checks before each seam remain active. Later checks
(permission errors, overwrite/update policy, interactive answers, install
content comparisons, and actual syscall failures) are not simulated. Recursive
copy/removal/move and parent creation are composite records, not expanded
per-syscall manifests. rmdir parent records remain conditional on emptiness at
execution; no pretend filesystem is maintained between operands. Fine-grained
backup names, full recursive expansion, and virtual filesystem simulation are
cut from this delivery under the epic KISS rule. The output names these scopes
and unevaluated decisions rather than claiming successful execution.

The default mutation bodies remain unchanged. No new dependencies, provider
calls, shell-outs, short flags, or certification options were added.

## Verification

- Focused `TestEffect*`: 17 mutation scenarios snapshot contents, modes,
  modification times, sizes, and tree membership; stdin panics if consumed.
  Includes write failure, reused contexts, underscore alias and false values.
- Whole packages: `go test ./tool ./cmds/{rm,mv,cp,ln,chmod,chown,touch,mkdir,rmdir,truncate,install}`.
- Vet/typecheck the same scope for darwin, windows, and linux. Full-tree
  crossvet and heavy suites are intentionally deferred off Dragon.
- Cert golden: exact unknown-flag stderr/status for both spellings on all 11
  applets; help SHA256 fixtures captured from 8c2706f2. Separate before/after
  multicall binaries compare status/stdout/stderr for help, version, no operands,
  and both dry-run spellings (55 invocations).
- Required remote bash-5.3 + POSIX/VSC acceptance gate is pending S2+S7 freeze,
  per the operator's explicit instruction. No POSIX/VSC run on Dragon.
