# `cut` POSIX Issue 7 locale and byte-preservation audit

This audit is deliberately limited to the public `cut` implementation and
tests in this directory. Its normative source is the POSIX.1-2016 Issue 7
[`cut` utility specification](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html).

## Normative reading

The `OPTIONS` clause defines `-c` over character positions and `-b` over byte
positions, with `-n` forbidding a byte range from splitting a character. The
`-d delim` operand-argument names a single character, which in a multibyte
locale may occupy more than one byte. `LC_CTYPE` "determine[s] the locale for
the interpretation of sequences of bytes of text data as characters", so the
character boundaries behind `-c`, `-n`, and `-d` are a property of the
invocation locale, not of any fixed encoding. `LC_ALL`, category-specific
`LC_CTYPE`, and `LANG` precedence follows POSIX Chapter 8 through
`pkg/locale.Resolve`; an unset environment deliberately selects `POSIX`.
Selected content is emitted in source order, once per position, joined by the
input delimiter in field mode; nothing in the specification permits
re-encoding the selected bytes. The `OPERANDS`, `STDIN`, `STDERR`, and
`EXIT STATUS` clauses require ordered file/`-` processing, diagnostics, and a
greater-than-zero status on failure.

## Implemented certification surface

The implementation (issue 736; the character model in `ctype.go` mirrors the
bounded model `cmds/fold` established for issue 735 without importing it)
selects character spans over the original line bytes; locale decoding
establishes boundaries only and output is always an exact source-byte slice.
It has no host shell-out, libc locale dependency, or process locale mutation.

| Surface | Focused evidence |
| --- | --- |
| C, POSIX, and unset environment treat every byte as one character: `-c` selects exact byte positions and `-n` is a no-op | `TestIssue736CharacterSpansFollowLCCType`, `TestIssue736ByteNoSplitFollowsLCCType`, `TestCutBytesAndChars` |
| C/POSIX UTF-8 aliases select whole multibyte characters under `-c` and adjust `-b -n` ranges to character boundaries, carrying original encoded bytes | `TestIssue736CharacterSpansFollowLCCType`, `TestIssue736ByteNoSplitFollowsLCCType`, `TestCutBytesNoSplit` |
| The carried `de_DE.ISO-8859-1` aliases use one-byte character boundaries and preserve bytes such as `0xe4` | `TestIssue736CharacterSpansFollowLCCType`, `TestIssue736ByteNoSplitFollowsLCCType` |
| Bytes malformed for the selected UTF-8 locale are one-byte character units whose original bytes reach stdout — never the U+FFFD encoding | `TestIssue736MalformedBytesAreNeverReencoded` |
| `-d delim` accepts exactly one character of the selected locale, including multibyte UTF-8 delimiters used for splitting and as the default output delimiter | `TestIssue736MultibyteDelimiter` |
| A `delim` that is not one character in the selected locale (including invalid UTF-8 bytes) is a usage error before any output | `TestIssue736DelimiterMustBeOneCharacterInLocale`, `TestCutUsageErrors` |
| `LC_ALL` > `LC_CTYPE` > `LANG` > POSIX fallback, with empty values falling through | `TestIssue736LCCTypePrecedence` |
| Unsupported `LC_CTYPE` is diagnosed with status 1 before stdin is read or an operand is opened, in every selection mode | `TestIssue736UnsupportedLocaleFailsBeforeInput`, `TestIssue736UnsupportedLocaleFailsBeforeOpeningOperand` |
| A final line missing the text-file trailing newline — including one ending in a multibyte or malformed sequence — is processed with exact bytes and no invented newline | `TestIssue736TextFileBoundaries`, `TestCutFields` |
| Lines longer than the internal read buffer keep contiguous character spans | `TestIssue736LongLineCharacterSelection` |
| File and `-` operands are processed in order; an operand error is diagnosed while later operands continue | `TestCutFiles` |
| Standard-output write failure reaches stderr with status 1 | `TestCutStandardOutputWriteError` |
| List grammar (numbered from 1, no `0`, no decreasing range) and mode exclusivity remain covered | `TestCutUsageErrors`, `TestCutFields`, `TestCutBytesAndChars` |

## Deliberate boundaries and residuals

This is a bounded pure-Go locale implementation, not a claim to support every
locale name installed on a host. It accepts bare `C`/`POSIX`, their UTF-8
codeset aliases, and the repository's `de_DE.ISO-8859-1` certification
aliases. Other effective `LC_CTYPE` values fail before operand I/O rather
than silently using C or UTF-8 semantics. Extending that set requires carried
character-boundary data plus command-specific byte tests.

Bytes that are malformed for the selected UTF-8 locale are outside the POSIX
text-file domain; preserving and counting each such byte as one character
position is the deterministic extension evidenced above. Diagnostics are
English-only; no translated message catalogs are shipped. `--complement`,
`-O`/`--output-delimiter`, `-z`/`--zero-terminated`, and
`-w`/`--whitespace-delimited` are GNU/BSD extensions retained and byte-tested
but not claimed as POSIX requirements; `-w` splits on the fixed blanks
`<space>`/`<tab>` regardless of locale. This audit does not promote the
command's ledger verdict to `verified`: the translated-diagnostics residual
and the bounded locale inventory remain.
