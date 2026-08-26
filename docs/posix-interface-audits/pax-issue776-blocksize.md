# pax default-blocksize POSIX Issue 7 audit (Sprint 79 issue 776 / Profile C)

Scope: the normative default physical-block-size language in POSIX Issue 7,
2016 Edition, for `pax -w` — specifically the `-b blocksize` option and the
per-format defaults given in the `-x format` operand. Authoritative reference,
the Open Group `pax` utility page:

<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html>

No licensed conformance-suite material was used; the decision rests entirely on
the public normative text quoted below.

## Normative text (quoted verbatim)

`-b blocksize`:

> Block the output at a positive decimal integer number of bytes per write to
> the archive file. Devices and archive formats may impose restrictions on
> blocking. Blocking shall be automatically determined on input. Conforming
> applications shall not specify a blocksize value larger than 32256. Default
> blocking when creating archives depends on the archive format.

`-x format` per-format defaults:

> **cpio** … The default blocksize for this format for character special
> archive files shall be 5120.
> **pax** … The default blocksize for this format for character special
> archive files shall be 5120.
> **ustar** … The default blocksize for this format for character special
> archive files shall be 10240.

## Exact normative scope

The three mandated defaults are each qualified **"for character special
archive files"**. POSIX gives *no* default block size for any archive whose
selected output is not character-special, such as a regular file or pipe. For
those the default is implementation-defined. Standard output is not a separate
file type: when it is the selected archive output, its underlying file may be
regular, a pipe, or character-special. The required value is keyed by format:

| `-x` format | character-special default | other sinks |
| ----------- | ------------------------- | ----------- |
| `pax` (default) | **5120** | implementation-defined |
| `ustar`         | 10240    | implementation-defined |
| `cpio`          | 5120     | implementation-defined |

## Finding: confirmed Bashy-owned gap (pax + character-special)

Before this change `defaultBlockSize` returned 10240 for both `pax` and
`ustar` regardless of sink type. For regular files and pipes that is a legal
implementation-defined choice and is unchanged. For a **character-special**
archive output writing the **pax** format it was wrong: POSIX mandates 5120,
not 10240. `ustar` (10240) and `cpio` (5120) already matched the device mandate.

This is a genuine gap and not a spec ambiguity: the pax device default is an
explicit `shall`, and rule 3 (upstream semantics are immutable — "same
default") binds it.

## Distinguishing the sink without suite material

`pax` learns the sink type from the selected output object. A named archive is
opened first and its `Stat` result is inspected, avoiding a pathname TOCTOU.
With no `-f`, or with `-f -`, the standard-output writer is inspected when it
supports `Stat`; the standalone multicall supplies the real `os.Stdout` file.
An embedding that supplies an abstract writer with no file mode retains the
implementation-defined non-device default.

## Behavior after the fix (fail-closed, exact)

- Default computed in `run()` stays the implementation-defined value
  (`defaultBlockSize`: pax/ustar 10240, cpio 5120).
- In `writeMode`, once the actual selected output is available, a sink that
  reports character-special with no explicit `-b` lowers the default
  (`charSpecialBlockSize`: pax/cpio 5120, ustar 10240).
- If a selected file-like sink reports a `Stat` error, `pax` fails before the
  first archive write and closes an archive it opened itself.
- An explicit `-b` always wins for every sink — the override is a default, not
  a clamp.

## Tests (`issue776_blocksize_test.go`)

- `TestCharSpecialDefaultBlockSize` — pax→5120, ustar→10240, cpio→5120 for
  opened named sinks and for both stdout spellings. The named pathname does
  not exist, while the opened sink reports character-special, discriminating
  actual-sink inspection from pre-open pathname inspection.
- `TestRegularFileDefaultBlockSize` — pax to a real regular file and to a
  stdout writer reporting regular both stay 10240.
- `TestCharSpecialExplicitBlockSizeWins` — `-b 512` is honored verbatim for
  named and stdout character-special sinks (3072-byte logical archive, no
  padding to 5120).
- `TestArchiveSinkStatFailureIsFailClosed` — inspection failure writes no
  archive bytes and closes a named sink without closing caller-owned stdout.
- `TestBlockSizeSelectors` — pins the two pure selectors to the exact values.

The single-member archive is 3072 logical bytes, so each lane emits exactly one
physical block and 5120 vs 10240 cannot be confused. Reverting the
`charSpecialBlockSize` pax value to 10240 makes the device tests fail — verified
during development.

Scope of the change: `cmds/pax` only. No shared or generated manifests touched.
