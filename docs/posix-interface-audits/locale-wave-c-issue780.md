# Locale wave C: awk numeric radix and expr BRE back-references

Issue: Coreutils Weave #780

Scope is source-only. This change does not alter the shared POSIX manifest,
generated guide, or applet matrices.

## Normative behavior checked

- POSIX Issue 7 `awk` assigns `LC_NUMERIC` to the radix used for numeric input,
  numeric/string conversion, and numeric output. Program numeric constants and
  command-line assignments retain the portable period spelling.
  <https://pubs.opengroup.org/onlinepubs/9699919799/utilities/awk.html>
- POSIX Issue 7 BRE `\n` matches the same string as the corresponding earlier
  subexpression. `expr`'s `:` operator anchors that BRE at the start and returns
  the first captured subexpression when one is present.
  <https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap09.html>
  <https://pubs.opengroup.org/onlinepubs/9699919799/utilities/expr.html>

The maintained GNU references were used only as black-box oracles. With
`POSIXLY_CORRECT=1` and German ISO-8859-1, GNU awk 5.4.1 converted input `4,5`
plus one to `5,5` and formatted the source constant `1.5` as `1,50`. GNU expr
10.0 returned the single byte `e9` from a doubled Latin-1 alpha captured by
`\([[:alpha:]]\)\1`, and selected `aa` for `\(a\|aa\)\1` against `aaaa`.

## Implementation boundary

- The existing GoAWK fork adds invocation-owned `interp.Config.DecimalPoint`.
  It tags input-derived numeric strings with their radix and localizes only
  numeric conversions (including string values converted to numbers), so
  literal output is untouched. Zero preserves the
  existing period behavior. Coreutils resolves `LC_ALL`, `LC_NUMERIC`, then
  `LANG`, accepts only `C`, `POSIX`, and the carried German ISO-8859-1 aliases,
  and rejects every other numeric locale before program-file or input reads.
- The locale byte BRE translator retains `\1` through `\9` and dispatches such
  patterns to the existing bounded, leftmost-longest matcher. Non-capturing
  groups used internally for byte-token alternatives do not change public
  capture numbering. Raw-byte offset decoding, locale classes, equivalence,
  and collation-range expansion remain on the same snapshotted tables.

Focused tests cover locale precedence, decimal input/print/printf, portable
source and `-v` assignment spelling, fail-closed unsupported locales, Latin-1
class/range back-references, unequal-byte rejection, capture offsets, and
leftmost-longest selection.
