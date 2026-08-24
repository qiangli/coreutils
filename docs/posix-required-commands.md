# POSIX-required command interfaces for Profiles C/D

Generated from the canonical machine-readable manifest
`docs/posix-required-commands.tsv` by `scripts/posix_manifest.py`.

The manifest is limited to the 116 required names configured by the
VSC-PCTS2016 POSIX08 Commands & Utilities scenario. Its syntax and
interface fields are curated from POSIX.1 Issue 7, 2016 Edition; the
generator deliberately never extracts options from help or prose.

## Ownership baseline

| Effective Profile C/D owner | Count |
| --- | ---: |
| Go-selected | 78 |
| Shell-selected | 22 |
| Pinned external provider | 16 |
| Required names | 116 |

There are 86 same-name Go implementations available. Eight are
intentionally shadowed by the shell (`echo`, `false`, `kill`, `printf`,
`pwd`, `test`, `true`, and the `time` keyword), so availability must not
be confused with effective ownership.

## TSV contract

`syntax_forms`, `operands`, `environment`, and `required_effects` are
semicolon-separated normalized interface facts. `required_options` contains
only explicit option tokens; `option_arguments` maps those tokens to required
arguments. A single `-` means none. `parser_model` records flag-set, manual,
custom, shell, keyword, and external parsing explicitly.

`clause_ids` identify applicable XCU sections. `evidence_ids` bind each row
to the 2016 POSIX page and selected implementation source. `specified` means
the interface has been recorded, not that behavioral conformance is proved.

Run `python3 scripts/posix_manifest.py --check` to fail on stale generated
documentation, denominator/owner drift, duplicate or malformed option tokens,
wrapped synopsis fragments, prose labels in interface facts, and missing or
command-mismatched clauses or evidence.

## Effective owner index

| Command | Go available | Shell selected | Effective owner | Parser | Evidence state |
| --- | :---: | :---: | --- | --- | --- |
| `alias` | no | yes | shell | `shell_builtin` | `specified` |
| `ar` | no | no | pinned provider | `external` | `specified` |
| `at` | yes | no | Go | `flagset` | `specified` |
| `awk` | yes | no | Go | `custom` | `specified` |
| `basename` | yes | no | Go | `flagset` | `specified` |
| `batch` | yes | no | Go | `flagset` | `specified` |
| `bc` | no | no | pinned provider | `external` | `specified` |
| `bg` | no | yes | shell | `shell_builtin` | `specified` |
| `cat` | yes | no | Go | `flagset` | `specified` |
| `cd` | no | yes | shell | `shell_builtin` | `specified` |
| `chgrp` | yes | no | Go | `flagset` | `specified` |
| `chmod` | yes | no | Go | `flagset` | `specified` |
| `chown` | yes | no | Go | `flagset` | `specified` |
| `cksum` | yes | no | Go | `flagset` | `specified` |
| `cmp` | yes | no | Go | `flagset` | `specified` |
| `comm` | yes | no | Go | `flagset` | `specified` |
| `command` | no | yes | shell | `shell_builtin` | `specified` |
| `cp` | yes | no | Go | `flagset` | `specified` |
| `crontab` | yes | no | Go | `flagset` | `specified` |
| `csplit` | yes | no | Go | `flagset` | `specified` |
| `ctags` | no | no | pinned provider | `external` | `specified` |
| `cut` | yes | no | Go | `flagset` | `specified` |
| `date` | yes | no | Go | `flagset` | `specified` |
| `dd` | yes | no | Go | `custom` | `specified` |
| `df` | yes | no | Go | `flagset` | `specified` |
| `diff` | yes | no | Go | `flagset` | `specified` |
| `dirname` | yes | no | Go | `flagset` | `specified` |
| `du` | yes | no | Go | `flagset` | `specified` |
| `echo` | yes | yes | shell | `shell_builtin` | `specified` |
| `ed` | no | no | pinned provider | `external` | `specified` |
| `env` | yes | no | Go | `flagset` | `specified` |
| `ex` | no | no | pinned provider | `external` | `specified` |
| `expand` | yes | no | Go | `flagset` | `specified` |
| `expr` | yes | no | Go | `custom` | `specified` |
| `false` | yes | yes | shell | `shell_builtin` | `specified` |
| `fc` | no | yes | shell | `shell_builtin` | `specified` |
| `fg` | no | yes | shell | `shell_builtin` | `specified` |
| `file` | yes | no | Go | `flagset` | `specified` |
| `find` | yes | no | Go | `custom` | `specified` |
| `fold` | yes | no | Go | `flagset` | `specified` |
| `getconf` | yes | no | Go | `flagset` | `specified` |
| `getopts` | no | yes | shell | `shell_builtin` | `specified` |
| `grep` | yes | no | Go | `flagset` | `specified` |
| `hash` | no | yes | shell | `shell_builtin` | `specified` |
| `head` | yes | no | Go | `flagset` | `specified` |
| `iconv` | yes | no | Go | `flagset` | `specified` |
| `id` | yes | no | Go | `flagset` | `specified` |
| `jobs` | no | yes | shell | `shell_builtin` | `specified` |
| `join` | yes | no | Go | `flagset` | `specified` |
| `kill` | yes | yes | shell | `shell_builtin` | `specified` |
| `ln` | yes | no | Go | `flagset` | `specified` |
| `locale` | yes | no | Go | `flagset` | `specified` |
| `localedef` | no | no | pinned provider | `external` | `specified` |
| `logger` | yes | no | Go | `flagset` | `specified` |
| `logname` | yes | no | Go | `flagset` | `specified` |
| `lp` | no | no | pinned provider | `external` | `specified` |
| `ls` | yes | no | Go | `flagset` | `specified` |
| `m4` | no | no | pinned provider | `external` | `specified` |
| `mailx` | no | no | pinned provider | `external` | `specified` |
| `make` | no | no | pinned provider | `external` | `specified` |
| `man` | no | no | pinned provider | `external` | `specified` |
| `mesg` | yes | no | Go | `flagset` | `specified` |
| `mkdir` | yes | no | Go | `flagset` | `specified` |
| `mkfifo` | yes | no | Go | `flagset` | `specified` |
| `more` | yes | no | Go | `flagset` | `specified` |
| `mv` | yes | no | Go | `flagset` | `specified` |
| `newgrp` | yes | no | Go | `flagset` | `specified` |
| `nice` | yes | no | Go | `flagset` | `specified` |
| `nm` | no | no | pinned provider | `external` | `specified` |
| `nohup` | yes | no | Go | `flagset` | `specified` |
| `od` | yes | no | Go | `flagset` | `specified` |
| `paste` | yes | no | Go | `flagset` | `specified` |
| `patch` | no | no | pinned provider | `external` | `specified` |
| `pathchk` | yes | no | Go | `flagset` | `specified` |
| `pax` | yes | no | Go | `flagset` | `specified` |
| `pr` | yes | no | Go | `flagset` | `specified` |
| `printf` | yes | yes | shell | `shell_builtin` | `specified` |
| `ps` | yes | no | Go | `flagset` | `specified` |
| `pwd` | yes | yes | shell | `shell_builtin` | `specified` |
| `read` | no | yes | shell | `shell_builtin` | `specified` |
| `renice` | yes | no | Go | `flagset` | `specified` |
| `rm` | yes | no | Go | `flagset` | `specified` |
| `rmdir` | yes | no | Go | `flagset` | `specified` |
| `sed` | yes | no | Go | `custom` | `specified` |
| `sh` | no | yes | shell | `shell_builtin` | `specified` |
| `sleep` | yes | no | Go | `flagset` | `specified` |
| `sort` | yes | no | Go | `flagset` | `specified` |
| `split` | yes | no | Go | `flagset` | `specified` |
| `strings` | yes | no | Go | `flagset` | `specified` |
| `strip` | no | no | pinned provider | `external` | `specified` |
| `stty` | yes | no | Go | `custom` | `specified` |
| `tabs` | yes | no | Go | `flagset` | `specified` |
| `tail` | yes | no | Go | `flagset` | `specified` |
| `talk` | no | no | pinned provider | `external` | `specified` |
| `tee` | yes | no | Go | `flagset` | `specified` |
| `test` | yes | yes | shell | `shell_builtin` | `specified` |
| `time` | yes | yes | shell | `shell_keyword` | `specified` |
| `touch` | yes | no | Go | `flagset` | `specified` |
| `tput` | yes | no | Go | `flagset` | `specified` |
| `tr` | yes | no | Go | `flagset` | `specified` |
| `true` | yes | yes | shell | `shell_builtin` | `specified` |
| `tsort` | yes | no | Go | `flagset` | `specified` |
| `tty` | yes | no | Go | `flagset` | `specified` |
| `umask` | no | yes | shell | `shell_builtin` | `specified` |
| `unalias` | no | yes | shell | `shell_builtin` | `specified` |
| `uname` | yes | no | Go | `flagset` | `specified` |
| `unexpand` | yes | no | Go | `flagset` | `specified` |
| `uniq` | yes | no | Go | `flagset` | `specified` |
| `uudecode` | yes | no | Go | `flagset` | `specified` |
| `uuencode` | yes | no | Go | `flagset` | `specified` |
| `vi` | no | no | pinned provider | `external` | `specified` |
| `wait` | no | yes | shell | `shell_builtin` | `specified` |
| `wc` | yes | no | Go | `flagset` | `specified` |
| `who` | yes | no | Go | `flagset` | `specified` |
| `write` | yes | no | Go | `flagset` | `specified` |
| `xargs` | yes | no | Go | `manual` | `specified` |
