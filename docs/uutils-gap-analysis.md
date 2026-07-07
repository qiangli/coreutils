# uutils command/flag gap analysis

Generated from this checkout on 2026-07-07 by comparing:

- local multicall binary built from `./cmd/coreutils`
- uutils multicall binary built from `priorart/uutils-coretuils`

The comparison is based on command inventory plus each command's `--help` output. It is therefore a declared CLI surface comparison, not a behavioral conformance test. Flags with the same spelling may still differ semantically.

## Summary

| Measure | Count |
|---|---:|
| Local registered commands (`cmds/all`) | 94 |
| uutils commands (`coreutils --list`) | 79 |
| Commands present in both | 53 |
| uutils commands missing locally | 26 |
| Local commands not in uutils coreutils set | 41 |

The local implementation generally exposes smaller GNU subsets per command. It also uses long `--help` / `--version` as framework-wide options for most tools, while uutils commonly exposes short `-h` / `-V` aliases too. Commands with only that universal alias gap are: `basename`, `cat`, `dirname`, `link`, `pwd`, `sleep`, `tsort`, `unlink`, and `yes`. `true` and `false` do not expose the local framework help/version options.

## uutils commands missing locally

These commands exist in uutils but are not registered in local `cmds/all`:

`[`, `b2sum`, `basenc`, `cksum`, `csplit`, `dd`, `dir`, `dircolors`, `expand`, `expr`, `factor`, `fmt`, `fold`, `more`, `nl`, `numfmt`, `od`, `pathchk`, `pr`, `printf`, `ptx`, `shred`, `sum`, `test`, `unexpand`, `vdir`.

## Local commands outside uutils coreutils

These are local commands not present in the uutils coreutils command list:

`at`, `atq`, `atrm`, `awk`, `batch`, `browser`, `cal`, `chgrp`, `chmod`, `chown`, `clip`, `cmp`, `crontab`, `diff`, `duration`, `fetch`, `find`, `grep`, `gzip`, `hexdump`, `hostname`, `id`, `jq`, `ntp`, `sed`, `stat`, `strings`, `sync`, `tar`, `time`, `timeout`, `tokens`, `tree`, `tz`, `uname`, `uptime`, `watch`, `which`, `whoami`, `xargs`, `yc`.

## Missing uutils options on overlapping commands

Universal `-h` / `-V` aliases are omitted from this table so the command-specific gaps stand out.

| Command | uutils options missing locally | Local options not seen in uutils help |
|---|---|---|
| `base32` | `-D` | - |
| `base64` | `-D` | - |
| `comm` | `-z`, `--check-order`, `--nocheck-order`, `--output-delimiter`, `--total`, `--zero-terminated` | - |
| `cp` | `-H`, `-L`, `-P`, `-S`, `-T`, `-Z`, `-a`, `-b`, `-d`, `-g`, `-i`, `-l`, `-s`, `-t`, `-u`, `-x`, `--archive`, `--attributes-only`, `--backup`, `--context`, `--copy-contents`, `--debug`, `--dereference`, `--interactive`, `--link`, `--no-dereference`, `--no-preserve`, `--no-target-directory`, `--one-file-system`, `--parents`, `--preserve-default-attributes`, `--progress`, `--reflink`, `--remove-destination`, `--sparse`, `--strip-trailing-slashes`, `--suffix`, `--symbolic-link`, `--target-directory`, `--update` | - |
| `cut` | `-n`, `-w`, `-z`, `--output-delimiter`, `--zero-terminated` | - |
| `date` | `-I`, `-R`, `-f`, `-s`, `--debug`, `--file`, `--iso-8601`, `--resolution`, `--rfc-3339`, `--rfc-email`, `--set` | - |
| `df` | `-B`, `-H`, `-P`, `-T`, `-a`, `-i`, `-k`, `-l`, `-t`, `-x`, `--all`, `--block-size`, `--exclude-type`, `--inodes`, `--local`, `--no-sync`, `--output`, `--portability`, `--print-type`, `--si`, `--sync`, `--total`, `--type` | - |
| `du` | `-0`, `-A`, `-B`, `-D`, `-H`, `-L`, `-P`, `-S`, `-X`, `-k`, `-l`, `-m`, `-t`, `-v`, `-x`, `--count-links`, `--dereference`, `--dereference-args`, `--exclude`, `--exclude-from`, `--files0-from`, `--inodes`, `--no-dereference`, `--null`, `--one-file-system`, `--separate-dirs`, `--si`, `--threshold`, `--time`, `--time-style`, `--verbose` | - |
| `echo` | `-E`, `-e`, `-n` | - |
| `env` | `-0`, `-C`, `-S`, `-a`, `-f`, `-v`, `--argv0`, `--block-signal`, `--chdir`, `--debug`, `--default-signal`, `--file`, `--ignore-signal`, `--list-signal-handling`, `--null`, `--split-string` | - |
| `head` | `-z`, `--zero-terminated` | - |
| `join` | `-e`, `-j`, `-o`, `-z`, `--check-order`, `--header`, `--nocheck-order`, `--zero-terminated` | - |
| `ln` | `-L`, `-P`, `-S`, `-T`, `-b`, `-i`, `-n`, `-r`, `-t`, `--backup`, `--interactive`, `--logical`, `--no-dereference`, `--no-target-directory`, `--physical`, `--relative`, `--suffix`, `--target-directory` | - |
| `ls` | `-1`, `-B`, `-C`, `-D`, `-F`, `-G`, `-H`, `-I`, `-L`, `-N`, `-Q`, `-S`, `-T`, `-U`, `-X`, `-Z`, `-b`, `-c`, `-f`, `-g`, `-k`, `-m`, `-n`, `-o`, `-p`, `-q`, `-s`, `-t`, `-u`, `-v`, `-w`, `-x`, `--author`, `--block-size`, `--classify`, `--color`, `--context`, `--dereference`, `--dereference-command-line`, `--dereference-command-line-symlink-to-dir`, `--dired`, `--escape`, `--file-type`, `--format`, `--full-time`, `--group-directories-first`, `--hide`, `--hide-control-chars`, `--hyperlink`, `--ignore`, `--ignore-backups`, `--indicator-style`, `--kibibytes`, `--literal`, `--long`, `--no-group`, `--numeric-uid-gid`, `--quote-name`, `--quoting-style`, `--show-control-chars`, `--si`, `--size`, `--sort`, `--tabsize`, `--time`, `--time-style`, `--width`, `--zero` | - |
| `md5sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `mkdir` | `-Z`, `--context` | - |
| `mktemp` | `-q`, `-t`, `--quiet`, `--suffix` | - |
| `mv` | `-S`, `-T`, `-Z`, `-b`, `-g`, `-i`, `-t`, `-u`, `--backup`, `--context`, `--debug`, `--interactive`, `--no-target-directory`, `--progress`, `--strip-trailing-slashes`, `--suffix`, `--target-directory`, `--update` | - |
| `paste` | `-z`, `--zero-terminated` | - |
| `printenv` | `-0`, `--null` | - |
| `readlink` | `-q`, `-s`, `-v`, `-z`, `--quiet`, `--silent`, `--verbose`, `--zero` | - |
| `realpath` | `-E`, `-L`, `-P`, `-q`, `-z`, `--canonicalize`, `--logical`, `--physical`, `--quiet`, `--relative-base`, `--zero` | - |
| `rm` | `-I`, `-d`, `-g`, `-i`, `--dir`, `--interactive`, `--no-preserve-root`, `--one-file-system`, `--preserve-root`, `--progress` | - |
| `rmdir` | `--ignore-fail-on-non-empty` | - |
| `seq` | `-t`, `--terminator` | - |
| `sha1sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `sha224sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `sha256sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `sha384sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `sha512sum` | `-t`, `-w`, `-z`, `--ignore-missing`, `--quiet`, `--status`, `--strict`, `--text`, `--warn`, `--zero` | `-b`, `--binary` |
| `shuf` | `-o`, `-r`, `-z`, `--output`, `--random-seed`, `--random-source`, `--repeat`, `--zero-terminated` | - |
| `sort` | `-C`, `-M`, `-R`, `-S`, `-T`, `-d`, `-g`, `-i`, `-m`, `-z`, `--batch-size`, `--buffer-size`, `--check-silent`, `--compress-program`, `--debug`, `--dictionary-order`, `--files0-from`, `--general-numeric-sort`, `--ignore-nonprinting`, `--merge`, `--month-sort`, `--parallel`, `--random-sort`, `--random-source`, `--sort`, `--temporary-directory`, `--version-sort`, `--zero-terminated` | - |
| `split` | `-C`, `-e`, `-t`, `-x`, `--additional-suffix`, `--elide-empty-files`, `--filter`, `--hex-suffixes`, `--line-bytes`, `--separator`, `--verbose` | - |
| `tac` | `-b`, `-r`, `--before`, `--regex` | - |
| `tail` | `-F`, `-s`, `-z`, `--debug`, `--max-unchanged-stats`, `--pid`, `--retry`, `--sleep-interval`, `--use-polling`, `--zero-terminated` | - |
| `tee` | `-p`, `--output-error` | - |
| `touch` | `-a`, `-f`, `-m`, `-t`, `--no-dereference`, `--time` | - |
| `tr` | `-C`, `-t`, `--truncate-set1` | - |
| `truncate` | `-o`, `-r`, `--io-blocks`, `--reference` | - |
| `tty` | `-s`, `--quiet`, `--silent` | - |
| `uniq` | `-D`, `-z`, `--all-repeated`, `--group`, `--zero-terminated` | - |
| `wc` | `--files0-from`, `--total` | - |

## Operand/argument-form gaps implied by options

The `--help` comparison does not deeply parse positional argument grammars, but several missing flags correspond directly to missing invocation forms:

- `cp`, `ln`, and `mv` lack uutils/GNU target-directory forms using `-t DIRECTORY SOURCE...` and no-target-directory forms using `-T`.
- `sort`, `du`, and `wc` lack NUL-delimited file-list forms such as `--files0-from`.
- `comm`, `cut`, `head`, `join`, `paste`, `shuf`, `tail`, and `uniq` lack zero-terminated record modes.
- checksum commands lack uutils verification-output controls (`--status`, `--quiet`, `--strict`, `--warn`, `--ignore-missing`) and text/zero modes.
- `env` lacks uutils command-launch shaping arguments such as `--chdir`, `--argv0`, signal handling options, and `-S` split-string parsing.
