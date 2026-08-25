# `paste` POSIX.1-2016 interface audit (issue 738)

## Normative scope

This audit is pinned to POSIX.1 Issue 7, 2016 Edition:
[`paste`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html).
GNU compatibility is out of scope for certification; GNU-shaped extras
(`-z`, zero-operand stdin default, the `\b \f \r \v` escapes beyond the
POSIX-required `\n \t \\ \0`) are recorded as extensions only.

## Prior state

`docs/posix-interface-audits/go-batch-4.md` and
`go-evidence-closure-batch-2.md` classified `paste` `partial`, with the
following exact residuals against `cmds/paste/paste.go` and
`cmds/paste/paste_test.go` as they stood before this batch:

- `-d LIST` was decoded as UTF-8 unconditionally, and a test pinned that
  non-locale-aware behavior even though the OPERANDS clause makes LC_CTYPE
  normative for the LIST's delimiter characters.
- Repeated `-` under `-s` sits on a genuine OPERANDS/`-s` clause conflict;
  the implementation's resolution (every `-` operand shares one
  invocation-wide stdin reader) was silent and untested.
- The Issue 7 twelve-operand minimum, the `\\` escape, serial `\0` (no
  delimiter within a file's line), mid-file read errors, and stdout write
  failures were all untested.

The `-s` + empty-input-file bare-newline requirement, previously an
`implementation_gap`, was already fixed and evidenced by
`TestPasteSerial` before this batch (commits 9c85d14 / 76cc77d / 731e7cc,
confirmed in `go-evidence-closure-batch-2.md`); it is not revisited here.

## Fix applied in this batch

`cmds/paste/paste.go` gains a bounded, carried `LC_CTYPE` character model
(`resolveCharacterModel`, `characterSize`), the same shape used by
`cmds/fold`'s locale work (issue 735): `LC_ALL` \> `LC_CTYPE` \> `LANG` \>
the `POSIX` default, resolved through `pkg/locale.Resolve`. C/POSIX with no
codeset and the carried `de_DE.ISO-8859-1` alias split `-d LIST` one byte
per delimiter character; the C/POSIX `.UTF-8` aliases decode one Unicode
rune per delimiter character. Every delimiter element keeps its original
bytes — decoding only finds character boundaries, never transcodes. A
locale outside this carried set is diagnosed and fails with status 1
*before* `parseDelims` runs and therefore before any operand is opened
(`run` calls `resolveCharacterModel` immediately after flag parsing,
ahead of the existing `parseDelims` call and all `open` calls in
`pasteParallel`/`pasteSerial`).

This is a genuine behavior change for the previously-implicit default:
`-d` LIST bytes outside the C/POSIX locale's ASCII range are now split one
byte per delimiter unless the invocation's LC_CTYPE selects a UTF-8
codeset. `cmds/paste/paste_test.go`'s two multibyte-delimiter cases were
updated to set `LC_ALL=C.UTF-8` accordingly (they were previously passing
only because of the unconditional UTF-8 decode this batch removes).

## New evidence closing the remaining residuals

All in `cmds/paste/issue738_posix_test.go`, package-internal so the
read-error and write-error cases can drive `pasteParallel`/`pasteSerial`
directly with injected I/O:

| Residual | Test |
| --- | --- |
| Locale-aware `-d LIST` decoding (C/POSIX single-byte, `.UTF-8` multibyte, `de_DE.ISO-8859-1` single-byte), exact bytes preserved | `TestIssue738LocaleAwareDelimiterDecoding` |
| `LC_ALL` \> `LC_CTYPE` \> `LANG` \> POSIX default precedence, empty values fall through | `TestIssue738LCCTypePrecedence` |
| Unsupported `LC_CTYPE` fails with status 1 before any operand is opened | `TestIssue738UnsupportedLocaleFailsBeforeOpeningOperand` |
| Repeated `-` under `-s`: first `-` drains stdin, later `-` operands see EOF and produce a bare-newline line | `TestIssue738RepeatedDashUnderSerial` |
| Twelve-operand minimum, parallel and serial | `TestIssue738TwelveOperands` |
| `\\` escape (a literal backslash delimiter) | `TestIssue738BackslashEscapedDelimiter` |
| Serial `\0` (no delimiter within a file's joined line) | `TestIssue738SerialZeroDelimiter` |
| Injected mid-file (non-EOF) read error: diagnosed, status 1; parallel aborts remaining rows while already-written rows stand, serial skips only the failing file's line and continues | `TestIssue738InjectedReadErrorParallel`, `TestIssue738InjectedReadErrorSerial` |
| Stdout write/flush failure (hard failure and short write), parallel and serial: diagnosed, status 1 | `TestIssue738OutputWriteErrors` |

Every new test was run against the pre-fix source for the locale case and
failed as expected (`TestIssue738LocaleAwareDelimiterDecoding`'s UTF-8
cases and the two updated `TestPasteParallel` multibyte cases require the
signature change); the remaining tests exercise behavior already present
in `pasteParallel`/`pasteSerial`, which this batch does not otherwise
modify.

## Honest residuals and boundaries

This remains a bounded pure-Go locale implementation: C/POSIX, their
`.UTF-8` aliases, and `de_DE.ISO-8859-1` are the only carried `LC_CTYPE`
providers, matching `cmds/fold`. Other installed locale names fail loudly
rather than silently choosing C or UTF-8 semantics. Translated diagnostics
and `NLSPATH` message catalogs remain unimplemented, as for every command
in this repository's current wave. Zero operands defaulting to stdin
remains a documented GNU-shaped extension beyond the `file...` synopsis,
not a POSIX requirement.
