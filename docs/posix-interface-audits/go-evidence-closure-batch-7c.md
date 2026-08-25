# POSIX Go evidence closure: batch 7C

Batch 7C audits the Go-owned `unexpand`, `uudecode`, `uuencode`, `wc`,
`who`, and `write` interfaces against POSIX.1-2016 Issue 7. The authoritative
sources are the six Open Group pages linked from the corresponding rows in
`docs/posix-required-command-interfaces.tsv`. The audit does not promote a row
from package-test success alone; the shared ledger is reconciled separately.

## Disposition

| Command | Evidence closed | Remaining boundary |
| --- | --- | --- |
| `unexpand` | Required `-a` and `-t` parsing, repeating and explicit tab lists, `-t` implying all-run conversion, file/stdin routing, maximum-tab/minimum-space conversion, explicit-list termination, backspace handling, malformed-list rejection, output/error paths, and effective `LC_CTYPE` blank and display-column behavior. C/POSIX is byte-classified; UTF-8 covers wide, zero-width, and East Asian Ambiguous characters; supported single-byte locales use `pkg/ctype`. | Runtime lookup of arbitrary non-UTF-8 named locales is bounded by `pkg/ctype`; unavailable providers fail explicitly in POSIX mode. Translated diagnostics remain absent. |
| `uudecode` | Exactly zero or one input file when `POSIXLY_CORRECT` is present, retention of the GNU multi-file extension otherwise, historical and Base64 payloads, leading-text scanning, octal and symbolic header modes, pathname and `-o` routing, umask-independent access bits, resolved-target overwrite rules, non-writable target refusal, and non-fatal permission-setting failure. | Real filesystem credential, symlink, and permission behavior remains platform-dependent integration evidence; diagnostics are not localized. No additional Issue 7 functional gap was confirmed. |
| `uuencode` | Required operand grammar, historical and `-m` Base64 framing, line limits, padding/terminators, ordinary-pathname treatment of input `-`, explicit-file access bits, and stdin descriptor access bits. | An embedding that supplies stdin as an abstract `io.Reader` has no descriptor access bits and uses the documented `0666` extension fallback; the standalone descriptor path is covered. Diagnostics are not localized. No additional Issue 7 functional gap was confirmed. |
| `wc` | Default and selected count ordering, stdin and repeated `-` routing, named-file rows, totals, read/write failures, and nonzero aggregate status. C/POSIX counts bytes as characters; UTF-8 `-m` counts decoded characters and `-w` uses Unicode whitespace; supported single-byte locales use provider-backed `isspace`. | Runtime lookup of arbitrary non-UTF-8 named locales is bounded by `pkg/ctype`; unavailable providers fail explicitly when a selected count depends on `LC_CTYPE`. Translated diagnostics remain absent. GNU `-L`, `--files0-from`, and `--total` are outside this audit. |
| `who` | Corrected base/XSI split (`-mTu` are base; `-q`, `am i`, and `am I` are XSI), exact operand grammar, `-q` selector behavior, `-a`, dead-process exit data, `-T`, short mode, invocation-local `LC_TIME`/`TZ`, native Linux ABI checks, and stdout/error paths. | Live login-accounting records, terminal activity/message state, and platform ABI behavior remain integration boundaries. The bounded `LC_TIME` provider fails closed outside carried locales; diagnostics are not localized. No additional Issue 7 algorithmic gap was confirmed. |
| `write` | Exact operand grammar, session/terminal ownership and permission checks, multiple-login choice, routing/framing, alerts, canonical input framing, SIGINT handling, real Linux PTYs, write/close failures, and effective `LC_CTYPE` print/space/control classification. High-bit controls receive printable meta rendering and provider failures occur before delivery. | Runtime lookup of arbitrary non-UTF-8 named locales is bounded by `pkg/ctype`; unavailable providers fail explicitly in POSIX mode. Live login databases, credentials, controlling-terminal ownership, and non-Linux terminal implementations remain integration boundaries; diagnostics are not localized. |

## Confirmed fixes

Issue 7 specifies `uudecode [-o outfile] [file]`, while GNU mode supports a
list of input files. With `POSIXLY_CORRECT` present, including an empty value,
the implementation rejects a second operand before opening a source or
creating output. With the switch absent, the multi-file extension remains.

The locale correction resolves `LC_ALL` over the category variable over
`LANG`, without mutating process-global locale state. `unexpand`, `wc`, and
`write` now select their POSIX locale paths by presence of `POSIXLY_CORRECT`.
No implementation shells out.

The shared ledger also needs the corrected `who` applicability split: `-m`,
`-T`, and `-u` are base; `-q`, the additional selectors, and `am i`/`am I`
are XSI-shaded.

## Focused evidence for reconciliation

| Command | Focused evidence |
| --- | --- |
| `unexpand` | `TestUnexpandLeadingBlanks`; `TestUnexpandAllAndFile`; `TestUnexpandTabsImpliesAll`; `TestUnexpandBlanksBeyondLastStopUnchanged`; `TestUnexpandBackspaceDecrementsColumn`; `TestUnexpandRejectsBadTabs`; locale tests in `cmds/unexpand/locale_test.go` |
| `uudecode` | `TestPOSIXRejectsMultipleInputFiles`; `TestNonPOSIXDecodesMultipleInputFiles`; `TestHeaderPathnamesAndSymbolicModes`; `TestHeaderAndOutputOverrideStdoutDistinctions`; `TestChmodFailureIsWarningAndNonfatal`; `TestHeaderModeIgnoresUmask`; `TestDecodeRefusesResolvedTargetWithoutEffectiveWriteAccess` |
| `uuencode` | `TestEncodeKnownVectorFromStdin`; `TestEncodeFileAndMode`; `TestEncodeBase64AndModes`; `TestStandardInputFileModeComesFromFstat`; `TestErrors` |
| `wc` | `TestWcStdin`; `TestWcFile`; `TestWcMultipleAndTotal`; `TestWcCharsAndMaxLine`; `TestWcWordRules`; `TestWcErrors`; locale tests in `cmds/wc/locale_test.go` |
| `who` | `TestWhoOperands`; `TestWhoQuietIgnoresOtherOptions`; `TestWhoTExactNoOptionalComment`; `TestWhoAllIsExactAndTruthful`; `TestWhoLCtimeProviderAndFailClosedResidual`; `TestWhoBinaryABIBehavior` |
| `write` | `TestDeliversBannerBodyAndEOF`; `TestMultiLoginNoticeGoesToStdoutAndAlertsGoToControllingTerminal`; `TestTypedBELReachesTheRecipientAsAByte`; `TestCanonicalEOLAndNewlineBothDelimit`; `TestSIGINTWritesEOTReturnsSuccessAndLeaksNothing`; `TestPTYBackedWriteLinux`; locale tests in `cmds/write/locale_test.go` |

For `who`, the ledger base synopsis is `who [-mTu] [file]`, with required
options `-m;-T;-u` and XSI conditional forms for `[-abdHlprt]`, `-s`, `-q`,
`am i`, and `am I`. Platform-specific evidence files remain identified by
their exact test IDs during aggregate reconciliation.

## Scope controls

No external provider or GNU compatibility behavior is used as POSIX evidence.
Shared TSV/generated manifests and aggregate counts remain the responsibility
of the coordinated reconciliation wave.
