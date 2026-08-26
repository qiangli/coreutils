# POSIX Issue 7 audit: `cat` and `xargs`

This issue audits the two Go-owned interfaces against the 2016 Issue 7
utility pages and the canonical rows in `docs/posix-required-command-interfaces.tsv`.
The audit tests required Issue 7 behavior without disabling compatible GNU
extensions merely because `POSIXLY_CORRECT` is present.

## `cat`

The required option, operand, standard-input, byte-stream, output, continuation,
and status clauses are covered by the existing line/byte tests, injected read
errors, directory and dangling-symlink operands, `/dev/null`, FIFO-through-
symlink streaming, and same-file protection. `TestCatBrokenPipeHonorsInheritedSIGPIPE`
proves the invocation boundary exposed by `RunContext`. The Unix helper-process
test `TestCatProcessSIGPIPEBehavior` additionally uses a real broken pipe: the
default disposition produces a SIGPIPE wait status, while an ignored disposition
produces status 1 and a write diagnostic.

The row remains **partial**. Issue 7 marks `cat` asynchronous events as
`Default`; the focused process test proves the command behavior but does not
claim that every standalone multicall/platform path correctly detects an
inherited ignored disposition. Windows special-file behavior and diagnostic
localization also remain truthful platform/product residuals.

## `xargs`

The required option and operand forms, logical EOF after quote processing,
default and `-I`/`-L` input parsing, trailing-blank continuation, replacement
limits, command-size/environment accounting, sequential execution, `/dev/tty`
prompting, locale yes-expression precedence, utility lookup, inherited child
environment, output/error propagation, and 0/1-125/126/127 status classes are
covered by focused package tests. Injected standard-input failure proves that a
read error diagnoses and prevents execution. The controlling-terminal fixture
includes a negative response between affirmative responses and proves the
declined command is skipped. Two-batch child-255 and signal cases prove exactly
one diagnostic and the required stop consequence without overclaiming a
particular nonzero status in the portable 1-125 range.

Confirmed fixes in this issue include an explicit empty `-I` replacement state,
correct last-option-wins state for the mutually exclusive `-I`, `-L`, and `-n`
forms, and negative `-P` rejection. `POSIXLY_CORRECT` is read invocation-locally
by presence (including an empty value) only to select the Issue 7 explicit `-s`
rule: each generated command line must be **less than** `size`. Compatible GNU
extensions such as `-0` and long spellings remain accepted. The distinct
automatic `{ARG_MAX}-2048` ceiling says the combined argument and environment
lists shall **not exceed** that ceiling, so equality remains permitted there.
No process-global environment or locale state is changed.

The row remains **partial**. Remaining evidence includes arbitrary non-C
`LC_CTYPE`/`LC_COLLATE` input and yes-expression breadth, real controlling-TTY
prompt integration, and cross-platform exec/argument-limit providers. The
implementation is sequential by default as required; GNU `-P` remains an
extension. Missing translated catalogs or `NLSPATH` alone are not
treated as certification blockers under the Sprint 79 consolidated policy.

## References

- [`cat` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html)
- [`xargs` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html)
- [`sprint-79-consolidated.md`](sprint-79-consolidated.md)
