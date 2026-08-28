# Coreutils GoAWK fork

This directory contains the production sources from the MIT-licensed
`github.com/qiangli/goawk` commit `ab9c4869bc2d`, itself based on
`github.com/benhoyt/goawk`. Coreutils keeps the fork local so its POSIX awk
behavior is reproducible without an unpublished module commit.

Coreutils-specific changes add invocation-owned locale callbacks, locale
numeric-string classification for command-line assignments and `FILENAME`,
working-directory/environment routing for shell commands, dynamic record
separator changes on open streams, POSIX escape decoding in lexical ERE
constants, and left-associative chains of the `~` and `!~` operators. The
applet's integration regressions live in `cmds/awk`.
