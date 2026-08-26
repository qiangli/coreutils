# `tee`, `tsort`, and `uname` Issue 764 POSIX.1 Issue 7 Audit

Scope: the Open Group POSIX.1-2008 Issue 7, 2016 Edition interfaces for
exactly `tee`, `tsort`, and `uname`, audited from coreutils baseline
`899cef8a80433e25c48c79322d761b9b07e605cc`. GNU 9.11 compatibility is not
conformance evidence. The canonical inventory is
[`posix-required-command-interfaces.tsv`](../posix-required-command-interfaces.tsv),
and the disposition follows
[`sprint-79-consolidated.md`](sprint-79-consolidated.md): missing translated
catalogs or `NLSPATH` routing alone are not treated as blockers.

## Result

This audit closes confirmed command-local defects without promoting any row:

- Required Issue 7 option behavior is unchanged by the retained, non-conflicting
  extensions. Tests prove those extensions remain available when
  `POSIXLY_CORRECT` is present; none of these commands needs a mode seam.
- `tee` now recognizes nil-error short writes. Portable injected products
  prove open, write, short-write, and close failures for one output are
  diagnosed, set failure status, close opened outputs, and do not suppress
  standard output or another output file. An input read returning bytes and an
  error preserves those bytes before failing. A staged reader proves output is
  visible before EOF. Unix subprocess tests prove default SIGINT, default
  SIGPIPE, and `-i` SIGINT behavior at a real process boundary. File creation
  now uses the invocation's virtual umask instead of the host process mask;
  existing output-file modes remain unchanged.
- `tsort` no longer uses Go's broad Unicode whitespace definition. Its scanner
  now recognizes exactly space, tab, and newline as separators, preserving
  form-feed, vertical-tab, carriage-return, and non-ASCII whitespace inside an
  item. Input read/close failures and output errors/short writes are diagnosed
  and fail. Deterministic tests pin exact cycle diagnostics, continuation
  output, malformed-input status, and usage status.
- `uname` now diagnoses output errors and nil-error short writes. A selected
  required provider field that is empty fails instead of silently omitting the
  symbol. Injected provider tests pin probe failure and unavailable-field
  status, while deterministic assembly tests prove selector composition,
  deduplication, fixed order, default `-s`, exact `-a == -mnrsv`, and explicit
  extension placement.

All three canonical rows remain `partial`. The added evidence is substantial
but does not constitute runtime certification on every supported target.

## Normative coverage

### `tee`

Source: [Issue 7 `tee`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html).

| Clause area | Audited disposition |
| --- | --- |
| Synopsis/options | `tee [-ai] [file...]`; `-a` uses append semantics and `-i` ignores SIGINT. Retained `-p`, `--output-error`, long-option, and help/version extensions remain available, including with `POSIXLY_CORRECT`, because they do not conflict with the required forms. |
| Operands/files | A literal `-` is a pathname. Default opens use create/truncate and mode `0666` subject to the invocation's process/virtual umask; `-a` uses `O_APPEND`. A test exercises the required minimum of thirteen file operands. |
| Streaming | Standard input bytes are written to standard output and each opened file as soon as each read completes; a staged reader proves no wait for EOF. Bytes returned with a read error are copied before the diagnostic and failure status. |
| Per-output errors | Open failure skips only that operand. Write and short-write failure disable only that sink and continue all others. Every opened file is closed; close failures are diagnosed without preventing later closes. Real Linux `/dev/full` evidence remains in addition to the portable seam. |
| Signals | Unix subprocess products prove ordinary SIGINT and broken-pipe SIGPIPE retain their default dispositions; `-i` survives SIGINT and completes the copy. Signal handling is scoped to the invocation. |
| Status | Success is 0. Input, stdout, file open/write/close, and usage failures are non-zero; repository usage errors use 2. |

### `tsort`

Source: [Issue 7 `tsort`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html).

| Clause area | Audited disposition |
| --- | --- |
| Synopsis/options | `tsort [file]`; Issue 7 has no required options. The newer `-w` behavior and help/version aliases remain non-conflicting extensions even with `POSIXLY_CORRECT`. `--` remains the option terminator. |
| Input grammar | Space, tab, and newline separate the token stream. Every two tokens form a pair. Identical items declare presence; unequal items add a directed ordering edge. Other whitespace bytes remain in items. An odd final token is diagnosed and fails. |
| Input selection | Omitted file and `-` use standard input; one pathname is opened through `RunContext.Path`. Extra operands are usage errors. Open, read, and close failures are diagnosed and return failure before a purported successful ordering. |
| Ordering/cycles | Every item is written once. A lexicographic ready frontier deterministically chooses among unconstrained items. On a cycle, the exact implementation diagnostic names the input and cycle members, one cycle edge is removed, sorting continues, and final status is non-zero. Independent cycles are separately diagnosed. |
| Output/status | Output is one item per line. Output errors and nil-error short writes fail. Success is 0, graph/input/output errors are 1, and usage errors are 2. |

### `uname`

Source: [Issue 7 `uname`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html).

| Clause area | Audited disposition |
| --- | --- |
| Synopsis/options | `uname [-amnrsv]`; operands are rejected. GNU `-o/-p/-i`, long names, and help/version aliases remain non-conflicting extensions, including with `POSIXLY_CORRECT`. |
| Selection/order | No selectors means `-s`. Repeated selectors do not duplicate fields. Flag spelling order never affects output order: system name, node name, release, version, machine. `-a` is exactly `-mnrsv`; explicitly requested extension fields follow the five required fields. |
| Providers | Unix targets use `uname(2)` and collapse whitespace runs so one symbol cannot split the output record. Darwin augments only GNU extension fields. Windows uses `RtlGetVersion`, `os.Hostname`, and a GOARCH mapping. Other targets use honest Go target/runtime/hostname identifiers rather than selecting an unavailable Unix provider. A missing selected required value is a diagnosed provider failure. |
| Streams/status | Standard input is not read. Exactly one newline-terminated output record is written. Probe, provider-value, output-error, and short-write failures return 1; usage returns 2; success returns 0. |

## Truthful residuals

- `tee` signal products run on Unix and the real `/dev/full` product runs on
  Linux. Windows signal behavior and real close-error products on additional
  filesystems remain platform/integration residuals; the portable close seam
  is not represented as a certification-host filesystem failure.
- `tsort` now has complete command-local C/POSIX behavior for the tested input
  domain, but runtime filesystem/error products beyond the current host and
  the bounded scanner token limit remain implementation/platform residuals.
- `uname` has a real Darwin provider run and cross-build coverage for the
  other provider files, including the non-Unix fallback. Linux, Windows, BSD,
  AIX, Solaris, JS, and WASI provider behavior is not all runtime-evidenced in
  this workspace. The values themselves are
  implementation-defined, but non-empty selection, composition, output, and
  failure behavior are command-local evidence.
- Diagnostics are fixed English. This is recorded as a localization product
  gap, not used alone to block or promote any row.

## Gate record

The issue gate includes repeated default and presence-based POSIX focused
tests, race runs, native and relevant cross-target vet/build checks, manifest
render/check, applet matrix generation/check, structural applet coverage,
formatting, and `git diff --check`. Exact commands and results are recorded in
the accepting commit and review handoff.
