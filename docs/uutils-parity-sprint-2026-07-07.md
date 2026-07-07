# uutils option parity sprint update

Date: 2026-07-07

This records the sprint that narrowed option/flag/argument gaps between
`bashy`/this Go coreutils implementation and the MIT-licensed
`reference/uutils-coreutils` implementation.

The implementation work used `reference/uutils-coreutils` as the parity
reference. GNU coreutils source was not used for implementation.

## Conductor/Fleet Split

| Owner | Scope | Result |
|---|---|---|
| Euler | stream/text utilities | Implemented parity additions for `head`, `tail`, `paste`, `wc`, `tee`, `tty`, `uniq`, `expand`, `unexpand` |
| Dirac | filesystem/path utilities | Implemented parity additions for `ln`, `readlink`, `realpath` |
| Conductor | listing/system utilities and integration | Implemented parity additions for `ls`, `hostname`, `nproc`, `uname`, `uptime`, `users`, `groups`; integrated worker changes and verified command packages |

## Second Conductor/Fleet Run

The follow-up conductor run used `bashy weave` isolated workspaces and assigned
the remaining high-value gaps across the local fleet (`codex`, `claude`, `agy`,
`opencode`, `aider`). Merged results:

| Issue | Tool | Scope | Result |
|---|---|---|---|
| `#28` | `codex` | `df` | Added block-size, inode, filesystem-type, output-field, sync, filter, and total modes |
| `#29` | `opencode` | `id`, `who`, `pinky` | Added remaining aliases/options with deterministic cross-platform fallbacks |
| `#30` | `codex` | `ln` | Added backup/suffix, interactive replacement, and logical/physical handling |
| `#31` | `opencode` | `tail` | Replaced the follow-mode stub with polling-based `-f`, `-F`, `--retry`, `-s`, `--pid`, `--max-unchanged-stats`, `--use-polling`, and `--debug` behavior |
| `#32` | `codex` | `du` | Added focused support for `--files0-from`, `-0`/`--null`, excludes, block-size modes, and `--si` |

Rejected or killed work:

- `#25` (`agy`, `tail`) passed its narrow gate but only registered missing
  flags and returned `tool.NotSupported`; it was left unmerged.
- `#26` (`aider`, `ln`) was killed after repeatedly probing nonsensical paths.
- `#27` (`claude`, `du`) was killed after a long no-change run and replaced by
  the narrower `#32` task.

## Implemented Changes

| Command(s) | Added uutils-compatible support |
|---|---|
| `head` | `-z`, `--zero-terminated` |
| `tail` | `-z`, `--zero-terminated` |
| `paste` | `-z`, `--zero-terminated` |
| `wc` | `--files0-from=FILE`, `--total=auto|always|only|never` |
| `tee` | `-p`, `--ignore-pipe-errors`, `--output-error[=MODE]` |
| `tty` | `-s`, `--silent`, `--quiet` |
| `uniq` | `-z`, `-D`, `--all-repeated[=METHOD]`, `--group[=METHOD]` |
| `expand`, `unexpand` | `-U`, `--no-utf8` byte-column mode |
| `ln` | `-t`, `--target-directory`, `-T`, `--no-target-directory`, `-n`, `--no-dereference`, `-r`, `--relative`; follow-up additions `-L`, `--logical`, `-P`, `--physical`, `-b`, `--backup`, `-S`, `--suffix`, `-i`, `--interactive`, `-V` |
| `readlink` | `-q`, `--quiet`, `-s`, `--silent`, `-v`, `--verbose`, `-z`, `--zero` |
| `realpath` | `-E`, `--canonicalize`, `-L`, `--logical`, `-P`, `--physical`, `-q`, `--quiet`, `-z`, `--zero`, `--relative-base` |
| `hostname` | `-d`, `--domain`, `-f`, `--fqdn`, `-i`, `--ip-address`, `-s`, `--short`; `-h`/`-V` aliases |
| `nproc`, `groups`, `users` | `-h`/`-V` aliases where they do not conflict with command-specific flags |
| `uname` | `-v`, `--kernel-version`; `-h`/`-V` aliases |
| `uptime` | `-p`, `--pretty`, `-s`, `--since`; `-h`/`-V` aliases |
| `ls` | uutils surface coverage for remaining display/sort/link flags, including `-1`, `-B`, `-C`, `-D`, `-F`, `-G`, `-H`, `-I`, `-L`, `-N`, `-Q`, `-S`, `-T`, `-U`, `-V`, `-X`, `-Z`, `-b`, `-c`, `-f`, `-g`, `-k`, `-l`, `-m`, `-n`, `-o`, `-p`, `-q`, `-s`, `-t`, `-u`, `-v`, `-w`, `-x`, plus matching long options such as `--format`, `--sort`, `--zero`, `--file-type`, `--classify`, `--ignore`, `--ignore-backups`, `--group-directories-first`, `--numeric-uid-gid`, and symlink dereference modes |
| `tail` | Follow-up additions `-f`, `--follow[=descriptor|name]`, `-F`, `--retry`, `-s`, `--sleep-interval`, `--pid`, `--max-unchanged-stats`, `--use-polling`, `--debug` |
| `df` | Follow-up additions `-B`, `--block-size`, `-H`, `--si`, `-P`, `--portability`, `-T`, `--print-type`, `-V`, `-a`, `--all`, `-i`, `--inodes`, `-k`, `-l`, `--local`, `--no-sync`, `--sync`, `--output`, `-t`, `--type`, `-x`, `--exclude-type`, `--total` |
| `id` | Follow-up additions `-A`, `-P`, `-V`, `-Z`, `-a`, `--context`, `-h`, `--ignore`, `-p`, `-r`, `--real`, `-z`, `--zero` |
| `who` | Follow-up additions `-V`, `-h`, `-m`, `--writable` |
| `pinky` | Follow-up additions `-V`, `-i`, `--lookup`, `-q` |
| `du` | Follow-up additions `-0`, `--null`, `-B`, `--block-size`, `--exclude`, `--exclude-from`, `--files0-from`, `-k`, `-m`, `--si` |

## Current Option-Surface Snapshot

This snapshot was generated after rebuilding `./cmd/coreutils` to
`/private/tmp/coreutils` and comparing live `--help` output with
`reference/uutils-coreutils/target/release/coreutils`.

| Scope | Remaining gap |
|---|---|
| uutils commands missing locally, excluding bash builtins | none |
| uutils commands covered by bash builtins and intentionally ignored | `[`, `kill`, `printf`, `test` |
| overlapping non-builtin command option tokens | none |

The final comparison uses an artifact-aware option extractor that reads declared
option lines and ignores prose/table false positives, for example `.env-style`
in `env --help` and descriptive text in `pr --help`.

## Verification

Passed:

```sh
env GOCACHE=/private/tmp/coreutils-go-build go test ./cmds/... ./tool ./multicall
```

Follow-up targeted gate after the second conductor run:

```sh
go test ./cmds/df/ ./cmds/id/ ./cmds/who/ ./cmds/pinky/ ./cmds/ln/ ./cmds/tail/ ./cmds/du/ ./tool -count=1 -timeout=90s
```

Known non-command-suite issue:

```sh
env GOCACHE=/private/tmp/coreutils-go-build go test ./...
```

still fails in `pkg/skills` because `TestDetectAgent` observes the current
Codex environment (`clean env detected "codex"`). That failure is unrelated to
the coreutils command changes.
