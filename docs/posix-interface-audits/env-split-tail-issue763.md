# `env`, `split`, and `tail` Issue 763 POSIX.1 Issue 7 Audit

Scope: Open Group POSIX.1-2008 Issue 7, 2016 Edition interfaces for exactly
`env`, `split`, and `tail`, audited against repository baseline `899cef8`.
This is a POSIX Issue 7 required-interface audit, not a GNU 9.11 compatibility
pass; GNU behavior is relevant only where Bashy deliberately retains an
extension, and it is not conformance evidence. `POSIXLY_CORRECT` is treated by
presence, invocation-local, and non-POSIX extensions are preserved outside
POSIX mode.

## Result

The audit confirmed and fixed one observable implementation defect in `tail`
and closed a set of concrete observable residuals across all three commands
with focused behavioral tests. No `split` or `env` source change was required:
their audited C/POSIX-locale algorithms already meet Issue 7, and the remaining
residuals are truthful platform/locale integration boundaries.

- `tail`'s option-cluster scan (`scanOrder`) terminated at the boolean `-f`,
  so a mode letter clustered behind it — as in `-fn2` — was never seen. With
  both `-c` and `-n` present, the extension resolution "the last of `-c`/`-n`
  on the command line wins" then selected the wrong mode for clustered forms
  (for example `tail -c5 -fn2` incorrectly stayed in bytes mode). `-f` now
  carries no value and no longer ends the cluster scan, so the last mode letter
  is honored regardless of clustering. Separate-argument forms are unchanged.

All three canonical ledger rows remain `partial`. The added evidence closes the
audited observable branches below, but none of the three is a complete POSIX
certification run: each retains a genuine platform, signal, or locale residual
recorded under Residuals, and translated message catalogs are absent (a
localization product gap, which the Sprint 79 consolidated policy explicitly
does not treat as a standalone interface blocker).

## Normative coverage

### `env`

Source: [Issue 7 `env`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html).

| Area | Audited disposition |
| --- | --- |
| Synopsis and operands | `-i` (and its historical `-` first-operand synonym) start an empty environment; leading `NAME=VALUE` operands apply in order; the first non-assignment operand names the utility and every later operand is its argument. Option parsing stops at the first operand (`SetInterspersed(false)`), so a utility's own flags are never consumed by `env`. |
| First-`=` delimiter | Only the first `=` splits `NAME` from `VALUE`; an embedded `=` is data preserved byte-for-byte on output (`A=B=C`), and an empty-`NAME` assignment is still an assignment, not a utility. |
| Standard input | With no utility operand `env` does not read standard input (proved with an exploding reader that fails on any `Read`); with a utility, standard input is passed through unchanged. |
| Modified `PATH` lookup | The utility is searched using the environment `env` constructs. A zero-length `PATH` prefix and a wholly empty `PATH` both mean the working directory only, never a silent fallback to a built-in default path. An explicit path skips `PATH`. |
| Exit status | Utility status is returned verbatim; a utility found but not invokable is 126, a utility not found is 127, and a failure in `env` itself (an unreadable `--file`, an unusable `--chdir` target) lands in the 1-125 band, distinct from 126/127. Usage errors are exit 2 (documented repo-wide deviation within the `>0` latitude). |
| Streams | With a utility `env` writes nothing of its own to either stream; without one it writes each resulting `NAME=VALUE` and a newline (or NUL under the `-0` extension), diagnosing a stdout write failure with exit 1. |

### `split`

Source: [Issue 7 `split`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html).

| Area | Audited disposition |
| --- | --- |
| Synopsis and operands | Both required forms (`-l line_count` and `-b n[k|m]`) are covered; at most one `file` and one `name` operand are accepted; an omitted `file` or a `file` of `-` selects standard input; the default prefix is `x`. |
| Special token `-` | An explicit `-` `file` operand selects standard input exactly like an omitted operand, and the `name`/PREFIX operand still applies. |
| Suffix namespace | The default suffix length is two lowercase letters. Under `POSIXLY_CORRECT` the namespace is fixed at `xaa`..`xzz` and split fails after it is exhausted rather than inventing a wider suffix; GNU's auto-widening remains outside POSIX mode. |
| Output files | An empty input creates no output file in every mode. A partial final line (no trailing newline) is preserved byte-for-byte in the last file, and concatenating the outputs reproduces the input exactly. |
| Consequences of errors | A non-EOF input read failure, and a failed output-file open (a read-only destination directory, Unix), are each diagnosed on stderr and exit 1; the failed output open is diagnosed naming the output file, not a silent success. |
| Streams and status | Standard output is not used; diagnostics use stderr; 0 on success, `>0` on error. |

### `tail`

Source: [Issue 7 `tail`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html).

| Area | Audited disposition |
| --- | --- |
| Synopsis and operands | The one-file synopsis with `-c number`/`-n number` and default `-n 10` is covered; multiple operands are a documented extension with `==> name <==` headers; a missing operand is diagnosed, exits 1, and every remaining operand is still processed with its header. |
| Sign semantics | `+N` copies from byte/line `N` at origin 1, with the `-c +N` skip performed by reading so it works on a non-seekable pipe; unsigned or `-N` copies the last `N`. |
| Mode combination | When `-c` and `-n` are both given the last one on the command line wins (an extension resolution of a POSIX-exclusive combination), now correct for clustered forms such as `-c5 -fn2` after the `scanOrder` fix. |
| Follow and standard input | With `-f` and no operand (or `-`), a pipe/FIFO standard input is read once to EOF and `-f` is ignored per the Issue 7 rule, while a regular file redirected onto standard input is followed by descriptor. |
| Consequences of errors | A non-EOF input read failure is diagnosed and exits 1, distinct from the stdout write-failure branch which also diagnoses and exits 1. |

## Residuals

- **`env`** — On Windows a COMMAND that dies by a signal has no POSIX wait-status
  model, so `commandSignalOutcome` reports GNU's 125 with no signal for a
  standalone process boundary to re-raise; the Unix 128+N boundary and signal
  re-raise are the evidenced path. The signal-disposition and standalone
  process-boundary behavior remains a platform integration residual. Diagnostics
  are fixed English (no `LC_MESSAGES`/`NLSPATH` catalogs). `env` retains GNU
  extensions (`-u`, `-0`, `-C`, `-a`, `-S`, `-f`, `--ignore-signal`, `-v`) under
  their non-POSIX spellings.
- **`split`** — Multibyte-filename `{NAME_MAX}` accounting for the output-name
  namespace is not implemented, and translated diagnostics are absent; both are
  locale/platform integration boundaries. The `-n` chunk, `-C`/`--line-bytes`,
  `-d`/`-x` suffix, `--additional-suffix`, `--separator`, and `--verbose`
  behaviors are retained GNU extensions.
- **`tail`** — Under `-f` multiple operands are followed sequentially, not
  concurrently (a recorded extension deviation from GNU); descriptor-mode
  truncation is silent (the old offset is retained — POSIX leaves shrink/rename
  unspecified); a character-device standard input under `-f` takes the ignore
  path within the page's latitude; the stdin read-error diagnostic names the
  operand by its literal token `-` (format is unspecified by Issue 7, status 1
  is the contract). `-F`/`-q`/`-v`/`-z`/`--pid`/`--retry`/`--sleep-interval`/
  `--max-unchanged-stats`/`--debug`, multiplier suffixes, and the obsolescent
  leading `-NUM` rewrite are retained extensions. Translated diagnostics are
  absent.

## Verification

The following gates passed on the Darwin host:

- default `go test` and `go test -count=3` for `cmds/env`, `cmds/split`, and
  `cmds/tail`;
- `go test -race` for all three packages;
- focused Windows, Linux, and Darwin `go vet` (which cross-typechecks tests) of
  all three packages, exercising the `//go:build !windows` split output-error
  test on the Unix legs and its exclusion on Windows;
- `scripts/applet-test-coverage.sh` (PASS, 154 shipped packages) and
  `scripts/applet-matrix.py`, which refreshed the env/split/tail test-file and
  test counts;
- `git diff --check` (no whitespace defects) and `gofmt` clean on all changed
  Go files;
- an exact comparison of the canonical TSV renderer output against the
  regenerated `docs/posix-required-command-interfaces.md`: the only difference
  is the three target rows' `Evidence lanes`, and every cited `env`/`split`/
  `tail` `go_evidence` test ID is confirmed present in its package
  (`_validate_evidence` reports available and explicit for all three rows).

The official `scripts/posix_manifest.py --check`, `scripts/posix_manifest_test.py`,
and therefore the `scripts/crossvet.sh` wrapper remain red on a **pre-existing**
condition unrelated to this audit: the canonical `sh` row's shell semantic
evidence (`sh: partial state requires focused semantic evidence`). This was
confirmed identical on the clean baseline (13 failures, 5 errors both before and
after this change), it concerns the shell-owned `sh` row and not the Go-owned
`env`/`split`/`tail` rows, and this audit did not alter or launder it. The
target rows validate individually, their generated Markdown exactly matches the
canonical TSV, and the cross-vet compile legs the wrapper could not reach all
passed independently for the three packages.
