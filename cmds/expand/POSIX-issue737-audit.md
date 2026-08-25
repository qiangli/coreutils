# `expand` POSIX Issue 7 locale and byte-preservation audit

This audit is deliberately limited to the public `expand` implementation and
tests in this directory. Its normative source is the POSIX.1-2016 Issue 7
[`expand` utility specification](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html).

## Normative reading

The `STDOUT` clause requires each input line with every `<tab>` replaced by the
number of `<space>` characters needed to reach the next tab stop, with all
other bytes copied unchanged. Consequently, decoding an input byte as U+FFFD
and writing the UTF-8 encoding of U+FFFD is not a conforming preservation
strategy, in the C locale (where every byte is a character) no less than in a
multibyte one.

`LC_CTYPE` determines the interpretation of byte sequences as characters and
the display-column width each character advances; the column at which a `<tab>`
is replaced therefore depends on it. `LC_ALL`, category-specific `LC_CTYPE`,
and `LANG` precedence follows POSIX Chapter 8 through `pkg/locale.Resolve`; an
unset environment deliberately selects `POSIX`. A `<backspace>` decrements the
column counter (never below zero) and a `<newline>` resets it, per the ledger
transcription. The `OPERANDS`, `STDIN`, `STDERR`, and `EXIT STATUS` clauses
require ordered file/`-` processing, diagnostics, and a greater-than-zero
status on an input or output error.

## Implemented certification surface

The implementation keeps each character's original byte slice through column
counting and output; locale decoding is used only to establish boundaries and
display width, reusing the accepted `fold` character model. It has no host
shell-out, libc locale dependency, or process locale mutation.

| Surface | Focused evidence |
| --- | --- |
| C, POSIX, and unset environment treat every byte as a one-byte character, count it one column, and reproduce it exactly | `TestIssue737MalformedAndCLocaleBytesAreNeverReencoded` |
| Carried C/POSIX UTF-8 aliases preserve multibyte characters and count display columns (two-byte, wide=2, combining=0) | `TestIssue737LocaleCharacterBoundariesPreserveOriginalBytes`, `TestExpandWideRuneCountsDisplayColumns`, `TestExpandCombiningMarkIsZeroWidth` |
| The carried `de_DE.ISO-8859-1` aliases use one-byte character boundaries and preserve bytes such as `0xe4` | `TestIssue737LocaleCharacterBoundariesPreserveOriginalBytes` |
| Malformed UTF-8 bytes are never transcoded or dropped; each stays one byte counting one column | `TestIssue737MalformedAndCLocaleBytesAreNeverReencoded` |
| `-i` initial-region conversion, backspace column tracking, and past-last-stop single spaces retain exact bytes across locales | `TestIssue737InitialRegionAndBackspaceAcrossLocales`, `TestExpandInitialOnly`, `TestExpandBackspaceDecrementsColumn`, `TestExpandTabsBeyondLastStopBecomeSingleSpaces` |
| `-t` interval, explicit list, `/N` and `+N` extension forms, repeats, and blank separation | `TestExpandTabListIncrement`, `TestExpandTabListExtend`, `TestExpandRepeatedTabsAccumulate`, `TestExpandBlankSeparatedTabList`, `TestExpandParseTabStops`, `TestExpandRejectsBadTabs` |
| `LC_ALL` > `LC_CTYPE` > `LANG` > POSIX fallback | `TestIssue737LCCTypePrecedence` |
| Unsupported `LC_CTYPE` is diagnosed with status 1 before stdin is read or an operand is opened | `TestIssue737UnsupportedLocaleFailsBeforeInput`, `TestIssue737UnsupportedLocaleFailsBeforeOpeningOperand` |
| Exit-status contract: 0 success, 2 invalid tablist, 1 unsupported locale | `TestIssue737ExitStatusMatrix`, `TestIssue737UsageErrorPrecedesLocaleValidation` |
| File and `-` operands are processed in order; POSIX mode terminates on operand difficulty while the GNU-compatible default continues | `TestExpandOperandAccessFailureModes` |
| Injected read errors and short writes reach stderr and status 1 | `TestIssue737ReadAndShortWriteErrors`, `TestIssue737RunReportsReadAndShortWriteErrors`, `TestExpandStandardOutputWriteError` |

## Deliberate boundaries and residuals

This is a bounded pure-Go locale implementation, not a claim to support every
locale name installed on a host. It accepts bare `C`/`POSIX`, their UTF-8
codeset aliases, and the repository's `de_DE.ISO-8859-1` certification
aliases, matching the accepted `fold` model. Other effective `LC_CTYPE` values
fail before operand I/O rather than silently using C or UTF-8 semantics.
Extending that set requires carried character boundary and display-width data
plus command-specific byte tests.

GNU `-i`/`--initial` and the uutils-parity `-U`/`--no-utf8` are extensions,
retained and byte-tested but not claimed as POSIX requirements. `-U` forces
one-byte column counting even in a UTF-8 locale and does not bypass locale
validation. For C/POSIX UTF-8, display widths come from the pinned
`go-runewidth` Unicode tables; terminal-specific ambiguous-width preferences
are not modeled. Bytes that are malformed for the selected UTF-8 locale are
outside the POSIX text-file domain; preserving and counting each such byte as
one unit is the deterministic extension evidenced above. The invalid-tablist
exit status 2 is the documented repository deviation from the
greater-than-zero wording. Diagnostics are English-only; no translated
message catalogs are shipped. This audit is verified only by the Go tests
cited above, not by a PCTS run.
