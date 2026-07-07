# GNU coreutils vs uutils-coreutils reference comparison

Generated from this checkout on 2026-07-07 by comparing:

- `reference/gnu-coreutils`
- `reference/uutils-coreutils`

This report separates source-level command coverage from the command set exposed by the default-built uutils multicall binary. Those are not the same thing.

## Command Inventory

| Surface | Count | Source |
|---|---:|---|
| GNU all known commands | 109 | `reference/gnu-coreutils/build-aux/gen-lists-of-programs.sh --list-progs`, with `ginstall` normalized to `install` and internal `libstdbuf.so` excluded |
| GNU normal/default commands | 92 | `normal_progs` in GNU `gen-lists-of-programs.sh`, with `ginstall` normalized to `install` |
| uutils default-built commands | 79 | `reference/uutils-coreutils/target/release/coreutils --list` after `cargo build --release --bin coreutils` |
| uutils source-level commands | 110 | command crates in `reference/uutils-coreutils/src/uu`, plus the `[` alias and root `coreutils` multicall binary, excluding shared crate `checksum_common` |

## Are The Command Sets Exactly The Same?

No.

At source level, uutils has all 109 GNU command names from this GNU reference and one extra command: `more`.

For the default-built uutils multicall binary, the command set is smaller than GNU. It exposes 79 commands.

### Missing From Default uutils vs GNU all-known

`arch`, `chcon`, `chgrp`, `chmod`, `chown`, `chroot`, `coreutils`, `groups`, `hostid`, `hostname`, `id`, `install`, `kill`, `logname`, `mkfifo`, `mknod`, `nice`, `nohup`, `nproc`, `pinky`, `runcon`, `stat`, `stdbuf`, `stty`, `sync`, `timeout`, `uname`, `uptime`, `users`, `who`, `whoami`.

Default uutils also has one command not in GNU coreutils: `more`.

### Missing From Default uutils vs GNU normal/default

`chgrp`, `chmod`, `chown`, `groups`, `id`, `install`, `logname`, `mkfifo`, `mknod`, `nohup`, `nproc`, `stat`, `sync`, `uname`, `whoami`.

Default uutils has `df` even though GNU classifies it as conditional/build-if-possible, and it has the extra non-GNU command `more`.

## Option / Flag / Argument Surface

They are not exactly the same.

I could not build GNU coreutils in this environment because the checkout does not contain a generated `configure`, and required bootstrap tools are missing (`autoconf`, `automake`, and `help2man`). So option comparison here is based on:

- uutils runtime `COMMAND --help` from the built multicall binary
- GNU source-level option names from command C sources

That is enough to show the surfaces differ, but it is not a full behavioral conformance result. A definitive per-option report should be generated from both built binaries' `--help` output and then followed by behavioral tests.

Representative differences found:

| Command | GNU/source options not seen in default uutils help | uutils help options not seen in GNU/source extraction |
|---|---|---|
| `cp` | `--keep-directory-symlink`, `--mode` | `--progress`, `--preserve-default-attributes` |
| `cut` | `--no-partial`, `--trimmed`, `--whitespace-delimited`, `-F` | short `-h` / `-V` aliases |
| `dd` | many GNU operand modes such as `--append`, `--direct`, `--fdatasync`, `--fsync`, `--fullblock`, `--nocache`, `--noerror`, `--notrunc`, `--sparse`, `--swab` | short `-h` / `-V` aliases |
| `env` | `--debug` | `--file`, short `-h` / `-V` aliases |
| `ln` | `--directory`, `-d`, `-F` | short `-h` / `-V` aliases |
| `mv` | `--exchange`, `--no-copy` | `--progress` |
| `shred` | `--unlink` | short `-h` / `-V` aliases |
| `split` | `--unbuffered`, numeric short suffix forms seen in GNU source | short `-h` / `-V` aliases |
| `tail` | GNU source names include descriptor/follow count handling not matched by uutils help extraction | `--use-polling` |
| `tee` | `--warn` | short `-h` / `-V` aliases |
| `touch` | `--atime` | short `-V` alias |
| `tsort` | `-w` | short `-h` / `-V` aliases |

There are also broad systematic differences:

- uutils commonly exposes `-h` and `-V` short aliases for help/version.
- uutils includes non-GNU extensions in some commands, for example `--progress` on `cp`, `mv`, and `rm`, and `--random-seed` on `shuf`.
- The default uutils binary lacks whole GNU commands unless built with expanded feature sets.
- Same option spelling does not guarantee identical semantics; this report only compares the declared CLI surface.

## Bottom Line

uutils is not exactly identical to GNU coreutils in this reference checkout.

At source level, it is command-complete for the GNU command names in this tree and adds `more`. The default binary built from this checkout is not command-complete: 31 GNU all-known commands are absent from `coreutils --list`. Option/flag surfaces also differ; a true drop-in claim needs per-command conformance tests, not just matching command names.
