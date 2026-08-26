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
archive files"**. POSIX gives *no* default block size for any other sink — a
regular file, a pipe, or standard output. For those the default is
implementation-defined. So the only value POSIX *requires* is the one used when
the archive is a character-special device (classically a tape), keyed by
format:

| `-x` format | character-special default | other sinks |
| ----------- | ------------------------- | ----------- |
| `pax` (default) | **5120** | implementation-defined |
| `ustar`         | 10240    | implementation-defined |
| `cpio`          | 5120     | implementation-defined |

## Finding: confirmed Bashy-owned gap (pax + character-special)

Before this change `defaultBlockSize` returned 10240 for both `pax` and
`ustar` regardless of sink type. For regular files and stdout that is a legal
implementation-defined choice and is unchanged. For a **character-special**
`-f` sink writing the **pax** format it was wrong: POSIX mandates 5120, not
10240. `ustar` (10240) and `cpio` (5120) already matched the device mandate.

This is a genuine gap and not a spec ambiguity: the pax device default is an
explicit `shall`, and rule 3 (upstream semantics are immutable — "same
default") binds it.

## Distinguishing the sink without suite material

`pax` learns the sink type from the `-f` operand alone. A device archive is a
character-special file, detectable with a portable `os.Stat` +
`FileInfo.Mode()&os.ModeCharDevice`. Standard output (no `-f`, or `-f -`) is
never treated as a "character special archive file" for this purpose — there is
no path to stat and POSIX's clause is about the archive *file*. `/dev/null` is
a character-special device on every unix target and is used by the focused test
as a real, hermetic device sink.

## Behavior after the fix (fail-closed, exact)

- Default computed in `run()` stays the implementation-defined value
  (`defaultBlockSize`: pax/ustar 10240, cpio 5120).
- In `writeMode`, once the `-f` path is known, a character-special sink with no
  explicit `-b` lowers the default to the POSIX device value
  (`charSpecialBlockSize`: pax/cpio 5120, ustar 10240).
- An explicit `-b` always wins for every sink — the override is a default, not
  a clamp.

## Tests (`issue776_blocksize_test.go`, `//go:build unix`)

- `TestCharSpecialDefaultBlockSize` — pax→5120, ustar→10240, cpio→5120 written
  to a real character-special device (`/dev/null`), captured through the
  `openArchiveSink` seam so the emitted physical block size is observable.
- `TestRegularFileDefaultBlockSize` — pax to a regular file stays 10240; the
  discriminator against the device lane (same format, block size decided by
  sink type alone).
- `TestCharSpecialExplicitBlockSizeWins` — `-b 512` to a device is honored
  verbatim (3072-byte logical archive, no padding to 5120).
- `TestBlockSizeSelectors` — pins the two pure selectors to the exact values.

The single-member archive is 3072 logical bytes, so each lane emits exactly one
physical block and 5120 vs 10240 cannot be confused. Reverting the
`charSpecialBlockSize` pax value to 10240 makes the device tests fail — verified
during development.

Scope of the change: `cmds/pax` only. No shared or generated manifests touched.
