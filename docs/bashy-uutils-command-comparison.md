# bashy vs uutils command group comparison

Regenerated on 2026-07-07 (after the fileutils, textutils, and
shellutils parity batches) from:

- bashy: `../bashy/bin/bashy commands --json` (built against this tree)
- uutils: `reference/uutils-coreutils` source-level command surface captured in `docs/uutils-command-groups.md`

Both inventories are grouped with the same broad categories: Bash builtins, file utils, text utils, shell utils, and misc/extended support.

Important interpretation notes:

- The bashy inventory is a live command catalog: shell builtins, in-process coreutils tools, and front-door verbs.
- The uutils inventory is source-level: command crates in `reference/uutils-coreutils/src/uu`, plus the `[` alias and the `coreutils` multicall entrypoint.
- The groups are mutually exclusive inside each inventory. For bashy, `echo`, `false`, `pwd`, and `true` exist in the in-process coreutils layer too, but are counted as Bash builtins because builtins resolve first.
- Presence means the command name is served; per-command flag coverage is
  documented in `docs/commands.md` (implemented commands follow the GNU
  manual exactly for supported flags, with documented deviations noted
  per row). Every command formerly "only in uutils" was
  conformance-reviewed against the GNU manual/source and uutils before
  this regeneration.

## Summary

| Group | uutils count | bashy count | overlap | only uutils | only bashy |
|---|---:|---:|---:|---:|---:|
| Bash builtins | 8 | 61 | 8 | 0 | 53 |
| File utils | 32 | 36 | 32 | 0 | 4 |
| Text utils | 39 | 52 | 39 | 0 | 13 |
| Shell utils | 30 | 44 | 30 | 0 | 14 |
| Misc / extended | 1 | 58 | 0 | 1 | 58 |
| Total grouped command names | 110 | 251 | - | - | - |

Every uutils command name is now served by bashy, with one deliberate
exception: the `coreutils` multicall entrypoint itself (bashy is its own
front door; the multicall binary lives in this repo as `cmd/coreutils`).

## Side-by-side by group

| Group | uutils | bashy |
|---|---|---|
| Bash builtins | `[`, `echo`, `false`, `kill`, `printf`, `pwd`, `test`, `true` | `.`, `:`, `[`, `alias`, `bg`, `bind`, `break`, `builtin`, `caller`, `cd`, `command`, `compgen`, `complete`, `compopt`, `continue`, `declare`, `dirs`, `disown`, `echo`, `enable`, `eval`, `exec`, `exit`, `export`, `false`, `fc`, `fg`, `getopts`, `hash`, `help`, `history`, `jobs`, `kill`, `let`, `local`, `logout`, `mapfile`, `popd`, `printf`, `pushd`, `pwd`, `read`, `readarray`, `readonly`, `return`, `set`, `shift`, `shopt`, `source`, `suspend`, `test`, `times`, `trap`, `true`, `type`, `typeset`, `ulimit`, `umask`, `unalias`, `unset`, `wait` |
| File utils | `basename`, `chcon`, `chgrp`, `chmod`, `chown`, `cp`, `dd`, `df`, `dir`, `dircolors`, `dirname`, `du`, `install`, `link`, `ln`, `ls`, `mkdir`, `mkfifo`, `mknod`, `mktemp`, `mv`, `readlink`, `realpath`, `rm`, `rmdir`, `shred`, `stat`, `sync`, `touch`, `truncate`, `unlink`, `vdir` | `basename`, `chcon`, `chgrp`, `chmod`, `chown`, `clip`, `cp`, `dd`, `df`, `dir`, `dircolors`, `dirname`, `du`, `find`, `install`, `link`, `ln`, `ls`, `mkdir`, `mkfifo`, `mknod`, `mktemp`, `mv`, `readlink`, `realpath`, `rm`, `rmdir`, `shred`, `stat`, `sync`, `tar`, `touch`, `tree`, `truncate`, `unlink`, `vdir` |
| Text utils | `b2sum`, `base32`, `base64`, `basenc`, `cat`, `cksum`, `comm`, `csplit`, `cut`, `expand`, `fmt`, `fold`, `head`, `join`, `md5sum`, `more`, `nl`, `numfmt`, `od`, `paste`, `pr`, `ptx`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum`, `shuf`, `sort`, `split`, `sum`, `tac`, `tail`, `tee`, `tr`, `tsort`, `unexpand`, `uniq`, `wc` | `awk`, `b2sum`, `base32`, `base64`, `basenc`, `cat`, `cksum`, `cmp`, `comm`, `csplit`, `cut`, `diff`, `expand`, `fmt`, `fold`, `grep`, `gunzip`, `gzip`, `head`, `hexdump`, `join`, `jq`, `md5sum`, `more`, `nl`, `numfmt`, `od`, `paste`, `pr`, `ptx`, `sed`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum`, `shuf`, `sort`, `split`, `strings`, `sum`, `tac`, `tail`, `tee`, `tokens`, `tr`, `tsort`, `unexpand`, `uniq`, `wc`, `xargs`, `zcat` |
| Shell utils | `arch`, `chroot`, `date`, `env`, `expr`, `factor`, `groups`, `hostid`, `hostname`, `id`, `logname`, `nice`, `nohup`, `nproc`, `pathchk`, `pinky`, `printenv`, `runcon`, `seq`, `sleep`, `stdbuf`, `stty`, `timeout`, `tty`, `uname`, `uptime`, `users`, `who`, `whoami`, `yes` | `arch`, `at`, `atq`, `atrm`, `batch`, `cal`, `chroot`, `crontab`, `date`, `duration`, `env`, `expr`, `factor`, `groups`, `hostid`, `hostname`, `id`, `logname`, `ncal`, `nice`, `nohup`, `nproc`, `ntp`, `pathchk`, `pinky`, `printenv`, `runcon`, `seq`, `sleep`, `sntp`, `stdbuf`, `stty`, `time`, `timeout`, `tty`, `tz`, `uname`, `uptime`, `users`, `watch`, `which`, `who`, `whoami`, `yes` |
| Misc / extended | `coreutils` | `act`, `act-runner`, `agent`, `ast-query`, `browser`, `chat`, `check`, `commands`, `context`, `dag`, `docker`, `doctl`, `doctor`, `fetch`, `find-references`, `foreman`, `gh`, `git`, `graph`, `helm`, `kopia`, `kubectl`, `list-symbols`, `login`, `loom`, `mirror`, `ollama`, `podman`, `rclone`, `repo-map`, `run`, `schedule`, `sdlc`, `search-symbols`, `seaweedfs`, `secrets`, `self`, `skills`, `sphere`, `sprint`, `tessaro`, `verify`, `weave`, `web`, `zot` |

## Per-group differences

### Bash builtins

Only in uutils: none.

Only in bashy:

`.`, `:`, `alias`, `bg`, `bind`, `break`, `builtin`, `caller`, `cd`, `command`, `compgen`, `complete`, `compopt`, `continue`, `declare`, `dirs`, `disown`, `enable`, `eval`, `exec`, `exit`, `export`, `fc`, `fg`, `getopts`, `hash`, `help`, `history`, `jobs`, `let`, `local`, `logout`, `mapfile`, `popd`, `pushd`, `read`, `readarray`, `readonly`, `return`, `set`, `shift`, `shopt`, `source`, `suspend`, `times`, `trap`, `type`, `typeset`, `ulimit`, `umask`, `unalias`, `unset`, `wait`

### File utils

Only in uutils: none (gap closed 2026-07-07 — chcon, dd, dir, dircolors,
install, mkfifo, mknod, shred, vdir shipped).

Only in bashy:

`clip`, `find`, `tar`, `tree`

### Text utils

Only in uutils: none (gap closed 2026-07-07 — b2sum, basenc, cksum,
csplit, expand, fmt, fold, more, nl, numfmt, od, pr, ptx, sum, unexpand
shipped, GNU-conformance-reviewed against the uutils/GNU references).

Only in bashy:

`awk`, `cmp`, `diff`, `grep`, `gunzip`, `gzip`, `hexdump`, `jq`, `sed`, `strings`, `tokens`, `xargs`, `zcat`

### Shell utils

Only in uutils: none (gap closed 2026-07-07 — arch, chroot, expr,
factor, groups, hostid, logname, nice, nohup, nproc, pathchk, pinky,
runcon, stdbuf, stty, users, who shipped).

Only in bashy:

`at`, `atq`, `atrm`, `batch`, `cal`, `crontab`, `duration`, `ncal`, `ntp`, `sntp`, `time`, `tz`, `watch`, `which`

### Misc / extended

Only in uutils:

`coreutils` (the multicall entrypoint — served in this repo by
`cmd/coreutils`; bashy is its own front door)

Only in bashy:

`act`, `act-runner`, `agent`, `ast-query`, `browser`, `chat`, `check`, `commands`, `context`, `dag`, `docker`, `doctl`, `doctor`, `fetch`, `find-references`, `foreman`, `gh`, `git`, `graph`, `helm`, `kopia`, `kubectl`, `list-symbols`, `login`, `loom`, `mirror`, `ollama`, `podman`, `rclone`, `repo-map`, `run`, `schedule`, `sdlc`, `search-symbols`, `seaweedfs`, `secrets`, `self`, `skills`, `sphere`, `sprint`, `tessaro`, `verify`, `weave`, `web`, `zot`
