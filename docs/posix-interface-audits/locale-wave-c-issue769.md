# POSIX Issue 7 locale wave C: `tr`, `unexpand`, `uniq`, `wc`

This issue closes the Sprint 79 rank-2 residual for four of the thirteen
commands named in
[`sprint-79-consolidated.md`](sprint-79-consolidated.md): "Add non-C
multibyte `LC_CTYPE`/`LC_COLLATE`/`LC_NUMERIC` fixtures that discriminate
character boundaries, classes, equivalence, ranges, ordering, blanks,
widths, and numeric rendering for each named command." The normative
authority is POSIX.1 Issue 7, 2016 Edition; GNU extension behavior is not
certification evidence. Integration verification belongs to the
proprietary harness and is not claimed here.

## The shared model, and its limit

Three character universes are reachable from an invocation environment,
and every fixture below selects exactly one of them through `rc.Env`
alone — no process-global locale state is read or written.

| LC_CTYPE | Universe | Data source |
| --- | --- | --- |
| `C` / `POSIX` / unset | one byte is one character | built-in ASCII tables |
| another single-byte locale | one byte is one character | `pkg/ctype` (glibc `*_l` over dlopen) |
| a UTF-8 codeset, POSIX mode | one multi-byte sequence is one character | the standard library's Unicode tables |

The third row is the load-bearing limit of this wave and the reason every
row below stays **partial**. This repository has no multi-byte locale
provider: `pkg/ctype` accepts only `C`, `POSIX`, and the two reviewed
ISO-8859-1 aliases, and `pkg/collate` offers `strcoll` comparison for the
same two aliases and enumerates no equivalence class. Multi-byte character
classes are therefore answered from Unicode properties rather than from the
selected locale's `LC_CTYPE` data. The divergence is small but real and is
not papered over: glibc's `blank` class excludes the non-breaking spaces
U+00A0, U+2007 and U+202F that Unicode's `Zs` category includes, so
`unexpand` and `uniq` treat those three characters as blanks where glibc
would not. Closing that — and the `LC_COLLATE` items below — requires a
shared multi-byte locale provider outside the paths this issue owns, so
the gap is recorded with standing evidence rather than approximated
further. `TestTrCollationResidualIsRecorded` is the executable form of
that record.

Outside POSIX mode every command keeps its historical GNU-compatible byte
model and opens no provider, which is what the four
`...OutsidePOSIXMode...` / `...KeepsLegacy...` tests pin.

## `tr`

Clauses: `XCU:tr:OPERANDS` (`c-c`, `[:class:]`, `[=equiv=]`, `[c*n]`, and
the sentence "Multi-byte characters require multiple, concatenated escape
sequences of this type, including the leading <backslash> for each byte"),
`XCU:tr:OPTIONS` (`-c`/`-C`, `-d`, `-s`, `-t`), `XCU:tr:ENVIRONMENT_VARIABLES`
(`LC_CTYPE` — "the interpretation of sequences of bytes of text data as
characters ... and the behavior of character classes"; `LC_COLLATE` — "the
behavior of range expressions and equivalence classes"), `XCU:tr:STDOUT`,
and `XCU:tr:EXIT_STATUS`.

**Confirmed Bashy-owned defect, fixed.** `tr` resolved `LC_CTYPE`
unconditionally and handed every non-C name to the single-byte provider,
which accepts no UTF-8 codeset. On any ordinary host — where `LC_ALL` or
`LANG` names a UTF-8 locale — *every* `tr` invocation failed with status 2
and produced no output, including invocations naming nothing but ASCII.
`XCU:tr:EXIT_STATUS` permits a status greater than zero only "if an error
occurs". `TestTrUTF8LocaleIsUsable` is the regression.

**Implemented.** `tr` now operates on characters of the selected universe
rather than on bytes: `cmds/tr/charmodel.go` selects the universe and
`cmds/tr/multibyte.go` parses control strings for the multi-byte one. Set
expansion, ranges, classes, paired `[:upper:]`/`[:lower:]` case mapping,
`[c*n]`, `-c`, `-d`, `-s`, `-t` and the input/output transform are all
character-oriented there. Escapes are resolved to bytes first and only
then decoded as characters, so `\303\251` names one character exactly as
the standard requires, while a lone `\303` names one uninterpretable byte.
A byte that begins no character keeps its own identity through set
expansion, translation and output rather than collapsing into U+FFFD. The
single-byte universe keeps its 256-entry lookup tables, so the byte path
is unchanged in both behavior and cost.

Two deliberate refusals, both named rather than approximated: the
complemented domain of a multi-byte `LC_CTYPE` is unbounded, so `-c` never
enumerates it (`complementPrefix` produces only the prefix a translation
can consume), and a `[c*]` fill in string2 combined with a complemented
string1 has no computable repeat count and is refused through
`tool.NotSupported`.

Tests: `cmds/tr/locale_test.go#TestTrUTF8LocaleIsUsable`,
`#TestTrPOSIXMultibyteCharacterBoundaries`,
`#TestTrPOSIXMultibytePreservesUninterpretableBytes`,
`#TestTrPOSIXMultibyteOctalEscapesAreBytes`,
`#TestTrPOSIXMultibyteClasses`,
`#TestTrPOSIXMultibyteCaseTranslation`,
`#TestTrPOSIXMultibyteRanges`,
`#TestTrPOSIXMultibyteComplement`,
`#TestTrPOSIXMultibyteRepeatAndTruncate`,
`#TestTrPOSIXLocalePrecedenceSelectsCTypeCategory`,
`#TestTrOutsidePOSIXModeKeepsByteCharacters`,
`#TestTrCollationResidualIsRecorded`.

**Residual — the row stays `partial`.** `XCU:tr:OPERANDS` scopes its
definition of `c-c` to the POSIX locale and `XCU:tr:ENVIRONMENT_VARIABLES`
gives `LC_COLLATE` the behavior of range expressions and equivalence
classes. This implementation answers both from character values only: a
non-C `LC_COLLATE` changes nothing, and `[=c=]` holds exactly its own
character, where glibc's `en_US.UTF-8` would also admit the accented
forms. Multi-byte class membership comes from Unicode rather than locale
data, as described above. `source-complete` is **not** justified for the
`LC_COLLATE` clause; `implemented` is not claimed.

## `unexpand`

Clauses: `XCU:unexpand:OPTIONS` (`-a`, `-t tablist`),
`XCU:unexpand:ENVIRONMENT_VARIABLES` (`LC_CTYPE`), and
`XCU:unexpand:STDOUT` ("replacing the maximum eligible runs of spaces with
tabs according to the default or selected tab stops").

**No source change.** The accepted display-column and locale-blank model
already covers the required behavior; this issue adds the missing
discriminating evidence. New fixtures prove the `LC_ALL` > `LC_CTYPE` >
`LANG` precedence for the category that selects the universe (the accepted
tests exercised only `LC_ALL`), that a UTF-8 codeset reached through
`LANG` alone opens no single-byte provider, that wide characters advance
the column count so the same blank run reaches a tab stop under UTF-8 and
falls short of one in the C locale, that blanks beyond Latin-1 (U+2003 at
one column, U+3000 at two) are recognised with their own widths, that the
multi-byte interpretation survives a line longer than the internal read
buffer, and that no `LC_NUMERIC` convention reaches the `-t` tablist,
which `XCU:unexpand:ENVIRONMENT_VARIABLES` does not list.

Tests: `cmds/unexpand/locale_test.go#TestPOSIXLocalePrecedenceSelectsCTypeCategory`,
`#TestPOSIXUTF8WideCharactersAdvanceDisplayColumns`,
`#TestPOSIXUTF8LocaleBlanksBeyondLatin1`,
`#TestPOSIXUTF8CharacterBoundaryBeyondReadBuffer`,
`#TestPOSIXNumericLocaleDoesNotAlterTablist`.

**Residual — the row stays `partial`.** The multi-byte `<blank>` set and
the display widths are Unicode-derived, not `LC_CTYPE`-derived (the
U+00A0/U+2007/U+202F divergence above); display width additionally has no
POSIX definition and follows the East Asian Width property. The
single-byte provider remains linux/amd64 and linux/arm64 only.

## `uniq`

Clauses: `XCU:uniq:OPTIONS` — `-f fields` ("a field is the maximal string
matched by the basic regular expression `[[:blank:]]*[^[:blank:]]*`") and
`-s chars` ("Ignore the first *chars* characters when doing comparisons";
"If specified in conjunction with the `-f` option, the first *chars*
characters after the first *fields* fields shall be ignored. If the
*chars* option-argument specifies more characters than remain on an input
line, a null string shall be used for comparison") — and
`XCU:uniq:ENVIRONMENT_VARIABLES` `LC_CTYPE`, which is explicit here:
"Determine the locale for the interpretation of sequences of bytes of text
data as characters **and which characters constitute a <blank> in the
current locale**."

**Confirmed Bashy-owned defects, fixed.** Both halves of that `LC_CTYPE`
sentence were unimplemented: `-s` advanced by bytes, so under a multi-byte
locale it could stop inside a character and leave a continuation byte in
the comparison key, and the field scanner tested a hardcoded
`c == ' ' || c == '\t'`, so no locale could widen or narrow `<blank>`.
`cmds/uniq/uniq.go` now loads a `keyModel` from the invocation's
`LC_CTYPE` in POSIX mode and extracts the key in characters with the
locale's `<blank>` set; outside POSIX mode the byte model is kept
unconditionally and no provider is opened.

Tests: `cmds/uniq/locale_test.go#TestUniqPOSIXMultibyteSkipsCharacters`,
`#TestUniqPOSIXMultibyteFieldsUseLocaleBlanks`,
`#TestUniqPOSIXSkipsPastEndOfLineCompareNullString`,
`#TestUniqPOSIXSingleByteLocaleUsesProviderAndPrecedence`,
`#TestUniqPOSIXCLocaleOpensNoProvider`,
`#TestUniqLocaleProviderFailuresAreDiagnosed`,
`#TestUniqOutsidePOSIXModeKeepsByteKeys`.

**Residual — the row stays `partial`.** The multi-byte `<blank>` set is
Unicode-derived rather than `LC_CTYPE`-derived. `-i` and `-w` are GNU
extensions that Issue 7 does not define: they keep their ASCII-folding and
byte-counting semantics and are outside POSIX evidence. `LC_COLLATE` is
not among the variables `XCU:uniq:ENVIRONMENT_VARIABLES` lists, and
comparison stays byte-wise as the standard's "adjacent matching lines"
requires.

## `wc`

Clauses: `XCU:wc:OPTIONS` (`-c`, `-l`, `-m`, `-w`),
`XCU:wc:ENVIRONMENT_VARIABLES` (`LC_CTYPE`), and `XCU:wc:STDOUT`, whose
word count is over "a non-zero-length string of characters delimited by
white space".

**No source change.** The accepted character/word model already covers the
required behavior; this issue adds the missing discriminating evidence.
New fixtures prove the `LC_ALL` > `LC_CTYPE` > `LANG` precedence including
empty-value fall-through, that a UTF-8 codeset selected through `LANG`
alone opens no single-byte provider, that a multi-byte character split
across the internal read boundary is still exactly one character for `-m`
(the boundary is exercised at three offsets around the 4096-byte buffer),
that U+3000 delimits words under a UTF-8 `LC_CTYPE` and is three ordinary
bytes in the C locale, that the `-L` extension's column count follows the
effective locale for an East Asian Ambiguous character, and that no
`LC_NUMERIC` radix or grouping convention reaches the counts —
`XCU:wc:ENVIRONMENT_VARIABLES` lists no `LC_NUMERIC`, so the counts are
plain decimal in every locale.

Tests: `cmds/wc/locale_test.go#TestPOSIXLocalePrecedenceSelectsCTypeCategory`,
`#TestPOSIXUTF8CharacterBoundaryAcrossReadBuffer`,
`#TestPOSIXUTF8WordsUseLocaleWhiteSpace`,
`#TestPOSIXUTF8MaximumLineWidthUsesEffectiveLocale`,
`#TestPOSIXNumericLocaleDoesNotAlterCounts`.

**Residual — the row stays `partial`.** The multi-byte white-space set is
Unicode-derived rather than `LC_CTYPE`-derived. `-L` and `--files0-from`
are GNU extensions outside POSIX evidence, and `-L`'s display width has no
POSIX definition. The single-byte provider remains linux/amd64 and
linux/arm64 only.

## Claim states

No row is promoted. All four stay `partial`: each has at least one
command-specific clause answered from Unicode or character-value data
instead of from the selected locale's `LC_CTYPE`/`LC_COLLATE` data, and
the byte-derived integration verification gate is separately deferred.
Absence of translated message catalogs is not treated as a blocker, per
the Sprint 79 consolidated policy.

## References

- [`tr` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html)
- [`unexpand` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unexpand.html)
- [`uniq` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html)
- [`wc` Issue 7](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wc.html)
- [XBD Internationalization Variables](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/basedefs/V1_chap08.html)
- [`sprint-79-consolidated.md`](sprint-79-consolidated.md)
