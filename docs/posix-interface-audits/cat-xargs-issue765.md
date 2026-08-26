# POSIX Issue 7 audit: `cat` and `xargs`

This issue audits the two Go-owned interfaces against the 2016 Issue 7
utility pages and the canonical rows in `docs/posix-required-command-interfaces.tsv`.
GNU 9.11 behavior is evidence only where it remains outside POSIX mode.

## `cat`

The required option, operand, standard-input, byte-stream, output, continuation,
and status clauses are covered by the existing line/byte tests, injected read
errors, directory and dangling-symlink operands, `/dev/null`, FIFO-through-
symlink streaming, and same-file protection. `TestCatBrokenPipeHonorsInheritedSIGPIPE`
also proves the invocation boundary exposed by `RunContext`: an inherited
ignored SIGPIPE makes a closed-pipe write an error, while the default embedded
closed-pipe path completes without a diagnostic.

The row remains **partial**. Issue 7 marks `cat` asynchronous events as
`Default`; a standalone process-level test must still evidence the default
SIGPIPE wait status on a real pipeline. That is a multicall/process-boundary
product, not something an embedded `Tool.Run` test may simulate by killing its
host. Windows special-file behavior and diagnostic localization remain truthful
platform/product residuals, not blockers created solely by missing catalogs.

## `xargs`

The required option and operand forms, logical EOF after quote processing,
default and `-I`/`-L` input parsing, trailing-blank continuation, replacement
limits, command-size/environment accounting, sequential execution, `/dev/tty`
prompting, locale yes-expression precedence, utility lookup, inherited child
environment, output/error propagation, and 0/1-125/126/127 status classes are
covered by focused package tests. The child-255 and signal cases prove the
required stop-with-diagnostic consequence without overclaiming a particular
nonzero status in the portable 1-125 range.

Confirmed fixes in this issue are invocation-local `POSIXLY_CORRECT` handling
by presence (including an empty value), rejection of GNU-only `-0`, `-r`, `-P`,
`-d`, deprecated aliases, and long spellings in that mode, preservation of
those extensions outside it, explicit empty `-I` replacement state, POSIX
strict `< size` batching, and negative `-P` rejection. No process-global
environment or locale state is changed.

The row remains **partial**. Remaining evidence includes arbitrary non-C
`LC_CTYPE`/`LC_COLLATE` input and yes-expression breadth, real controlling-TTY
prompt integration, and cross-platform exec/argument-limit providers. The
implementation is sequential by default as required; GNU `-P` remains an
outside-mode extension. Missing translated catalogs or `NLSPATH` alone are not
treated as certification blockers under the Sprint 79 consolidated policy.

## References

- [`cat` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html)
- [`xargs` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html)
- [`sprint-79-consolidated.md`](sprint-79-consolidated.md)
