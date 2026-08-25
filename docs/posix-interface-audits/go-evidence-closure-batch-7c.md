# POSIX Go evidence closure: batch 7C

Batch 7C audits the Go-owned `unexpand`, `uudecode`, `uuencode`, `wc`,
`who`, and `write` interfaces against POSIX.1-2016 Issue 7. The authoritative
sources are the six Open Group pages linked from the corresponding rows in
`docs/posix-required-command-interfaces.tsv`. All six rows are proposed to
move from `unverified` to `partial` during the coordinated shared-TSV
reconciliation; this audit does not claim complete conformance from package-test
success alone.

## Disposition

| Command | Evidence closed | Remaining boundary |
| --- | --- | --- |
| `unexpand` | Required `-a` and `-t` parsing, repeating and explicit tab lists, `-t` implying all-run conversion, file/stdin routing, maximum-tab/minimum-space conversion at tab stops, explicit-list termination, backspace column handling, malformed-list rejection, and output/error status paths. | `LC_CTYPE` is not yet connected to the implementation. Only ASCII space and tab are classified as blanks, the default path treats every decoded rune as one display column, and the byte path is selected by a GNU extension rather than by the effective C/POSIX locale. Locale-defined blanks and wide or zero-width characters therefore remain Bashy-owned gaps. |
| `uudecode` | Exactly zero or one input file, historical and Base64 payloads, leading-text scanning, octal and symbolic header modes, pathname and `-o` routing (including the asymmetric `-`/`/dev/stdout` rules), umask-independent access bits, resolved-target overwrite rules, non-writable target refusal, and non-fatal permission-setting failure. | The fixed implementation has hermetic coverage for its required interface. Real filesystem credential, symlink, and permission behavior remains platform-dependent integration evidence; diagnostics are not localized. No additional Issue 7 functional gap was confirmed in this batch. |
| `uuencode` | Required operand grammar, historical and `-m` Base64 framing, 45-byte historical and 76-character Base64 line limits, padding/terminators, ordinary-pathname treatment of input `-`, explicit-file access bits, and stdin descriptor access bits. | An embedding that supplies stdin as an abstract `io.Reader` has no descriptor access bits and uses the documented `0666` extension fallback; the standalone file-descriptor path is covered. Diagnostics are not localized. No additional Issue 7 functional gap was confirmed. |
| `wc` | Default and selected count ordering, stdin and repeated `-` routing, named-file rows, multi-file totals, C/POSIX newline/word/byte/character counts, read/write failures, and nonzero aggregate status. | `LC_CTYPE` is not connected: `-m` is assigned the byte count and `-w` uses a fixed six-byte C-locale whitespace table. Multibyte character counts and locale-defined whitespace remain Bashy-owned gaps. The GNU `-L`, `--files0-from`, and `--total` extensions are outside this audit. |
| `who` | Corrected base/XSI split (`-mTu` are base; `-q`, `am i`, and `am I` are XSI), exact operand grammar, `-q` ignoring every other selector, exact `-a` expansion, mandatory dead-process exit data, `-T` field shape, short mode, invocation-local `LC_TIME`/`TZ`, native Linux ABI checks, and stdout/error status paths. | Live login-accounting records, terminal activity/message state, and platform ABI behavior cannot be established solely by hermetic fixtures. The bounded `LC_TIME` provider deliberately fails closed outside its carried locales, and diagnostics are not localized. No additional Issue 7 algorithmic gap was confirmed. |
| `write` | Exact no-option operand grammar, active-session and terminal ownership checks, message permission, deterministic multiple-login choice and stdout notice, recipient banner/body/`EOT` separation, two sender-terminal alerts, canonical NL/EOF/EOL framing, alert pass-through, control rendering, real Linux PTYs, short writes/close failures, and SIGINT success with `EOT`. | Unsupported non-UTF-8 `LC_CTYPE` providers fall back to C classification, so every valid locale's print/space classes are not covered. Live system login databases, credentials, controlling-terminal ownership, and non-Linux terminal implementations remain platform integration boundaries. |

## Confirmed fix

Issue 7 specifies `uudecode [-o outfile] [file]`, not a GNU-style list of
input files. The implementation previously iterated over every operand and
even advertised `[FILE]...`. It now rejects a second input operand before
opening any source or creating any decoded output. The focused regression is
`cmds/uudecode/uudecode_test.go#TestPOSIXRejectsMultipleInputFiles`.

The proposed shared-TSV reconciliation also corrects `who`'s applicability
split. Issue 7 defines `-m`, `-T`, and `-u` in the base interface; `-q`, the
additional selectors, and the special `am i`/`am I` forms are XSI-shaded.

## Proposed shared TSV reconciliation

The coordinator should move all six rows to `partial`, replace their
`UNVERIFIED` behavioral fields with the interface definitions summarized
above, and add these focused `go_evidence` references:

| Command | Proposed focused evidence |
| --- | --- |
| `unexpand` | `TestUnexpandLeadingBlanks`; `TestUnexpandAllAndFile`; `TestUnexpandTabsImpliesAll`; `TestUnexpandBlanksBeyondLastStopUnchanged`; `TestUnexpandBackspaceDecrementsColumn`; `TestUnexpandRejectsBadTabs` |
| `uudecode` | `TestPOSIXRejectsMultipleInputFiles`; `TestHeaderPathnamesAndSymbolicModes`; `TestHeaderAndOutputOverrideStdoutDistinctions`; `TestChmodFailureIsWarningAndNonfatal`; `TestHeaderModeIgnoresUmask`; `TestDecodeRefusesResolvedTargetWithoutEffectiveWriteAccess` |
| `uuencode` | `TestEncodeKnownVectorFromStdin`; `TestEncodeFileAndMode`; `TestEncodeBase64AndModes`; `TestStandardInputFileModeComesFromFstat`; `TestErrors` |
| `wc` | `TestWcStdin`; `TestWcFile`; `TestWcMultipleAndTotal`; `TestWcCharsAndMaxLine`; `TestWcWordRules`; `TestWcErrors` |
| `who` | `TestWhoOperands`; `TestWhoQuietIgnoresOtherOptions`; `TestWhoTExactNoOptionalComment`; `TestWhoAllIsExactAndTruthful`; `TestWhoLCtimeProviderAndFailClosedResidual`; `TestWhoBinaryABIBehavior` |
| `write` | `TestDeliversBannerBodyAndEOF`; `TestMultiLoginNoticeGoesToStdoutAndAlertsGoToControllingTerminal`; `TestTypedBELReachesTheRecipientAsAByte`; `TestCanonicalEOLAndNewlineBothDelimit`; `TestSIGINTWritesEOTReturnsSuccessAndLeaksNothing`; `TestPTYBackedWriteLinux` |

For `who`, the shared row specifically needs base synopsis
`who [-mTu] [file]`, required options `-m;-T;-u`, and XSI conditional forms
for `[-abdHlprt]`, `-s`, `-q`, `am i`, and `am I`. The current row incorrectly
labels `-q` as base and `-mTu` as XSI. Test identifiers above are relative to
each command package except `TestHeaderModeIgnoresUmask` and
`TestDecodeRefusesResolvedTargetWithoutEffectiveWriteAccess` in
`cmds/uudecode/uudecode_unix_test.go`, `TestWhoBinaryABIBehavior` in
`cmds/who/who_binary_posix_test.go`, and `TestPTYBackedWriteLinux` in
`cmds/write/pty_linux_test.go`.

## Scope controls

No implementation shells out. No external provider or GNU compatibility
behavior is used as POSIX evidence. Shared generated manifest Markdown,
aggregate counts, and the applet matrix are intentionally not regenerated by
this batch; their reconciliation belongs to the coordinated aggregate wave.
