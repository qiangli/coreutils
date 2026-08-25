# `fold` POSIX Issue 7 locale and byte-preservation audit

This audit is deliberately limited to the public `fold` implementation and
tests in this directory. Its normative source is the POSIX.1-2016 Issue 7
[`fold` utility specification](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html).

## Normative reading

The `DESCRIPTION` and `OPTIONS` require `fold` to insert newlines without
breaking a character, whether width is measured in display-column positions
or bytes (`-b`). The `STDOUT` clause requires the input character sequence and
order to remain unchanged apart from those inserted newlines. Consequently,
decoding an input byte as U+FFFD and writing the UTF-8 encoding of U+FFFD is not
a conforming preservation strategy.

`LC_CTYPE` determines the interpretation of byte sequences as characters and
the display-column width of each character. `LC_ALL`, category-specific
`LC_CTYPE`, and `LANG` precedence follows POSIX Chapter 8 through
`pkg/locale.Resolve`; an unset environment deliberately selects `POSIX`.
Backspace, carriage return, and tab use the special column rules stated in the
`DESCRIPTION`. Under `-s`, the break is after the last `<blank>` that meets the
specified width constraint. The `OPERANDS`, `STDIN`, `STDERR`, and
`EXIT STATUS` clauses require ordered file/`-` processing, diagnostics, and a
greater-than-zero status on an input or output error.

## Implemented certification surface

The implementation keeps each character's original byte slice through
counting and output; locale decoding is used only to establish boundaries and
column width. It has no host shell-out, libc locale dependency, or process
locale mutation.

| Surface | Focused evidence |
| --- | --- |
| C, POSIX, and unset environment treat every byte as a one-byte character and reproduce it exactly | `TestIssue735MalformedAndCLocaleBytesAreNeverReencoded` |
| C/POSIX UTF-8 aliases preserve valid multibyte characters in default, `-c`, and `-b` modes; `-b` counts encoded bytes without splitting a character | `TestIssue735LocaleCharacterBoundariesPreserveOriginalBytes`, `TestFoldBytesPreservesUtf8UnitsAtSmallWidth`, `TestFoldBytesCountsUTF8EncodingWidth` |
| The carried `de_DE.ISO-8859-1` aliases use one-byte character boundaries and preserve bytes such as `0xe4` | `TestIssue735LocaleCharacterBoundariesPreserveOriginalBytes` |
| Malformed UTF-8 bytes are never transcoded or dropped in default, `-c`, or `-b` modes | `TestIssue735MalformedAndCLocaleBytesAreNeverReencoded` |
| `-s`, tab, backspace, and carriage return retain exact bytes and obey the applicable column/byte rules | `TestIssue735SpacesAndControlCharactersAcrossLocales`, `TestFoldSpacesKeepsTrailingBlank`, `TestFoldSpacesKeepsLeadingBlanks`, `TestFoldTabAdvancesColumn`, `TestFoldBackspaceDecrementsColumn`, `TestFoldCarriageReturnResetsColumn` |
| `LC_ALL` > `LC_CTYPE` > `LANG` > POSIX fallback | `TestIssue735LCCTypePrecedence` |
| Unsupported `LC_CTYPE` is diagnosed with status 1 before stdin is read or an operand is opened | `TestIssue735UnsupportedLocaleFailsBeforeInput`, `TestIssue735UnsupportedLocaleFailsBeforeOpeningOperand` |
| File and `-` operands are processed in order; an operand error is diagnosed while later operands continue | `TestFoldPOSIXOperandsAndErrors` |
| Injected read errors and short writes reach stderr and status 1 | `TestIssue735ReadAndShortWriteErrors`, `TestIssue735RunReportsReadAndShortWriteErrors` |
| Required `-b`, `-s`, `-w width`, and obsolete `-width` syntax remain covered | tests above plus `TestFoldObsoleteWidthSyntax` and `TestFoldRejectsBadWidth` |

## Deliberate boundaries and residuals

This is a bounded pure-Go locale implementation, not a claim to support every
locale name installed on a host. It accepts bare `C`/`POSIX`, their UTF-8
codeset aliases, and the repository's `de_DE.ISO-8859-1` certification aliases.
Other effective `LC_CTYPE` values fail before operand I/O rather than silently
using C or UTF-8 semantics. Extending that set requires carried character
boundary and display-width data plus command-specific byte tests.

For C/POSIX UTF-8, display widths come from the pinned `go-runewidth` Unicode
tables. The implementation does not model terminal-specific ambiguous-width
preferences. Bytes that are malformed for the selected UTF-8 locale are
outside the POSIX text-file domain; preserving and counting each such byte as
one unit is the deterministic extension evidenced above. Diagnostics are
English-only; no translated message catalogs are shipped. GNU `-c` is an
extension, retained and byte-tested but not claimed as a POSIX requirement.
