# uutils command groups

Generated from `reference/uutils-coreutils` on 2026-07-07.

This uses the source-level uutils command surface: command crates under `reference/uutils-coreutils/src/uu`, excluding shared crate `checksum_common`, plus the `[` alias and the `coreutils` multicall entrypoint.

Total commands: 110.

The groups below are mutually exclusive. Commands that are also Bash builtins are listed only in the Bash builtins group.

## Bash builtins

Count: 8.

`[`, `echo`, `false`, `kill`, `printf`, `pwd`, `test`, `true`

## File utils

Count: 32.

`basename`, `chcon`, `chgrp`, `chmod`, `chown`, `cp`, `dd`, `df`, `dir`, `dircolors`, `dirname`, `du`, `install`, `link`, `ln`, `ls`, `mkdir`, `mkfifo`, `mknod`, `mktemp`, `mv`, `readlink`, `realpath`, `rm`, `rmdir`, `shred`, `stat`, `sync`, `touch`, `truncate`, `unlink`, `vdir`

## Text utils

Count: 39.

`b2sum`, `base32`, `base64`, `basenc`, `cat`, `cksum`, `comm`, `csplit`, `cut`, `expand`, `fmt`, `fold`, `head`, `join`, `md5sum`, `more`, `nl`, `numfmt`, `od`, `paste`, `pr`, `ptx`, `sha1sum`, `sha224sum`, `sha256sum`, `sha384sum`, `sha512sum`, `shuf`, `sort`, `split`, `sum`, `tac`, `tail`, `tee`, `tr`, `tsort`, `unexpand`, `uniq`, `wc`

## Shell utils

Count: 30.

`arch`, `chroot`, `date`, `env`, `expr`, `factor`, `groups`, `hostid`, `hostname`, `id`, `logname`, `nice`, `nohup`, `nproc`, `pathchk`, `pinky`, `printenv`, `runcon`, `seq`, `sleep`, `stdbuf`, `stty`, `timeout`, `tty`, `uname`, `uptime`, `users`, `who`, `whoami`, `yes`

## Misc

Count: 1.

`coreutils`
