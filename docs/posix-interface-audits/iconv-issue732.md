# `iconv` POSIX.1-2016 interface audit (issue 732)

Normative source: [POSIX.1-2016 `iconv`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html).
GNU behavior is not used as evidence. This audit supersedes the point-in-time
`iconv` claims in `go-batch-3.md` and `go-evidence-closure-batch-2.md`; later
work had already closed their `-c`, decoder-validation, charmap, `-s`, `-l`,
and CP858 findings.

## Verdict

The carried interface implements all four Issue 7 synopsis forms. The audit
found and closes two correctness defects: a short stdout write with a nil error
was accepted by the ordinary codeset path, and a charmap character absent from
the target map was advanced by one byte rather than by the complete source
character. The latter could incorrectly reinterpret a suffix byte as another
character.

The command remains `partial`, not `verified`, only at the explicit locale and
platform boundaries below. There is no known command-owned conversion,
operand, stream, or status residual within the carried encoding and locale
corpus.

## Clause-by-clause evidence

| Requirement | Implementation and focused evidence |
| --- | --- |
| Four synopses | `cmd.Usage` presents the exact `-f frommap -t tomap`, omitted-`-t`, omitted-`-f`, and standalone `-l` forms. `TestIssue732HelpShowsAllPOSIXSynopses`, `TestIssue723ListIsStandaloneAndEveryEntryRoundTrips`, and `TestOmittedEncodingUsesLocaleCodeset` pin them. Both `-f` and `-t` omitted matches no synopsis and is a usage error. |
| `-c` | Invalid input characters and characters not representable in the target are omitted, but conversion remains unsuccessful. `TestIssue732DiscardMalformedAndTruncatedSequences` exercises malformed/truncated UTF-8, UTF-16BE, Shift_JIS, EUC-JP, ISO-2022-JP, GBK, Big5, EUC-KR, HZ-GB-2312, and GB18030 with and without `-c`; `TestDiscardInvalidOmitsUntranslatableCharacters` covers target non-representability. Status is 1 in both modes. |
| `-s` | Only invalid/unrepresentable-character messages are suppressed. File-open, input-device, output-device, charmap-open/parse, and usage failures remain diagnostic and non-zero. Evidence: `TestIssue732DiscardMalformedAndTruncatedSequences`, `TestIssue732OpenFailureContinuesAndSilentDoesNotHideIt`, `TestIssue723SilentDoesNotSuppressInputIOErrors`, `TestIssue723SilentDoesNotSuppressOutputIOErrors`, `TestIssue732SilentDoesNotSuppressShortOutputWrite`, and `TestIssue723CharmapOperandFailuresAreNotSilent`. |
| `-f`/`-t` names and aliases | Names resolve through the carried x/text IANA registry plus explicit spelling aliases. `-l` reports every accepted carried value and every reported value round-trips in both roles (`TestIssue723ListIsStandaloneAndEveryEntryRoundTrips`, `TestEncodingAliases`, `TestIssue723CP858AliasesResolveToRealEncoding`). Unsupported names and non-standard `//IGNORE`/`//TRANSLIT` suffixes fail loudly. POSIX leaves the supported codeset-name set implementation-defined. |
| Omitted `-f` or `-t` | The omitted side derives from `LC_CTYPE`, using POSIX precedence (`LC_ALL`, category, `LANG`, implementation default). C/POSIX is US-ASCII; explicit `.CODESET`, C.UTF-8, and the carried unqualified de_DE mapping are supported; unknown unqualified locales fail closed. `TestIssue732LocalePrecedenceForOmittedEncoding`, `TestOmittedEncodingHonorsLocaleCodeset`, and `TestOmittedEncodingLocaleMappingAndFailClosed` pin the boundary. |
| Charmap operands | A slash selects the pathname form. Both valid charmaps are parsed and joined by symbolic character name, including custom escape/comment characters, decimal/octal/hex byte forms, ranges, aliases, and multibyte encodings. `TestIssue723CharmapPathnamesUseSymbolicJoin`, `TestIssue723CharmapSyntaxAndAliasJoin`, parser rejection tests, and `TestIssue732CharmapDropsWholeUnrepresentableMultibyteCharacter` cover it. POSIX makes results undefined for invalid or incomplete charmaps; this implementation rejects structurally invalid files loudly. |
| Input operands | No operands reads stdin; `-` denotes stdin; named operands are processed in order relative to the invocation directory; failures do not prevent later operands. `TestFilesResolveAgainstRunContextDir`, `TestIssue732FileAndStandardInputOperandsAreOrderedStreams`, and `TestIssue732OpenFailureContinuesAndSilentDoesNotHideIt` cover ordering, repeated `-`, continuation, and status. A repeated `-` naturally observes EOF after the first occurrence drains the same stream. |
| Streaming/state | Each input operand gets a fresh decoder, while all operands share one encoder and stdout stream. Independent UTF-16 files can each carry a BOM; stateful target prefixes are not restarted per file. `TestIssue732FileAndStandardInputOperandsAreOrderedStreams`, `TestDiscardStatusSurvivesLaterEmptyOperand`, and `TestIssue732ValidMultibyteCharactersCrossEveryReadBoundary` cover file boundaries and one-byte read boundaries for every carried multibyte family. |
| Malformed/truncated input | Strict validation precedes x/text's otherwise lenient decoders. Literal U+FFFD remains distinguishable from decoder replacement in UTF-8, UTF-16, and GB18030. The malformed matrix above plus `TestDiscardInvalidPreservesLiteralReplacementAndStreamingState` and `TestDiscardTruncatedGB18030FourByteTailFails` pin EOF and incremental behavior. |
| Standard streams and I/O failures | Converted bytes alone go to stdout; `-l` writes names; diagnostics go to stderr. Read, ordinary write, short-nil-write, charmap write, and list write failures produce status 1. `TestIssue723SilentDoesNotSuppressInputIOErrors`, `TestIssue723SilentDoesNotSuppressOutputIOErrors`, `TestIssue732SilentDoesNotSuppressShortOutputWrite`, and `TestIssue723ShortWritesFailForCharmapAndList` cover injectable device boundaries. |
| Status | 0 means every requested conversion and I/O operation succeeded; syntax errors return 2; unsupported encodings, conversion loss, operand errors, and I/O errors return 1. In particular, neither `-c` nor `-s` launders status. |
| Environment | `LANG`, `LC_ALL`, and `LC_CTYPE` affect omitted encoding selection through the invocation-local environment. `LC_MESSAGES` and `NLSPATH` are not interpreted because translated message catalogs are not carried. Stdin is otherwise unused by `-l`; stdout/stderr are as above. |

## Honest boundaries

- The supported codeset-name set is implementation-defined by POSIX and is
  deliberately limited to the values emitted by `-l`; aliases accepted in
  addition are also listed. No host `iconv` or OS conversion service is used.
- POSIX only mandates charmap operands on systems advertising the
  Locale-Definition option. The repo's Darwin `getconf` surface advertises
  `_POSIX2_LOCALEDEF`, and the pure-Go symbolic-join implementation supplies
  that form without a host utility. Invalid or incomplete charmap input has
  undefined POSIX results and is rejected where detected.
- Locale codeset discovery is based on a deterministic carried corpus, not
  host `nl_langinfo(CODESET)`. Explicit locale names containing a supported
  `.CODESET` work; unknown unqualified locale names fail rather than silently
  choosing an encoding.
- `LC_MESSAGES` translation and `NLSPATH` catalogs are not implemented; all
  diagnostics are deterministic English. This is the remaining reason for a
  `partial` verdict.
- POSIX does not specify output after a conversion error when `-c` is absent.
  This implementation omits malformed source characters and continues, but
  stops on a target representation error; both paths report failure. With
  `-c`, the required omission-and-continue behavior is deterministic.
