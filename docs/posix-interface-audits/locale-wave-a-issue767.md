# Locale Wave A Issue 767

Scope is limited to `cmds/awk`, `cmds/comm`, `cmds/csplit`, `cmds/expr`, and
`cmds/fold`, reconciled against POSIX.1 Issue 7, 2016 Edition. GNU extensions
are not closure evidence.

## awk

- **Clause/source:** XCU `awk` `ENVIRONMENT VARIABLES` lists `LANG`,
  `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `LC_NUMERIC`, `NLSPATH`,
  and `PATH`. Regex character classes, equivalence classes, collating elements,
  and ranges are governed by XBD 9.3.
- **Tests:** `TestAwkLocaleRegexEveryEndpoint`,
  `TestAwkResolvesCTypeAndCollateIndependently`, `TestAwkLocaleLifecycle`,
  `TestAwkLocaleEquivalenceClassMatches`, plus the existing POSIX format and
  ERE tests in `awk_test.go` and `awk_routing_test.go`.
- **Confirmed state:** LC_CTYPE and LC_COLLATE are invocation-owned and wired
  through the shared byte-regexp locale tables for GoAWK regex entry points.
- **Honest residual:** LC_NUMERIC remains command-visible but not implemented.
  The exact focused red is `LC_NUMERIC=de_DE.UTF-8 awk 'BEGIN { print 1.5 }'`;
  POSIX requires the period character to represent the radix character in
  program source, but numeric output and string conversion are affected by the
  selected locale. GoAWK exposes no command-local numeric-radix hook, and this
  repository has no shared LC_NUMERIC provider analogous to LC_TIME. No shared
  provider or vendored interpreter change was made in this packet.
- **Source-complete:** no.

## comm

- **Clause/source:** XCU `comm` `ENVIRONMENT VARIABLES` lists `LANG`,
  `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and `NLSPATH`.
  The merge and sorted-input assumptions use LC_COLLATE.
- **Tests:** `TestCommUsesInvocationCollatorForMergeAndOrderChecks`,
  `TestCommLocaleInitFailsBeforeInputOpen`, `TestCommCAndPOSIXBypassCollator`,
  plus existing stream, order, diagnostic, and output-failure tests.
- **Confirmed state:** command-local behavior is source-complete for the carried
  LC_COLLATE provider surface; C/POSIX remains bytewise and non-C setup fails
  before operand I/O when the provider is unavailable.
- **Honest residual:** installed locales outside the bounded provider corpus
  fail closed; translated diagnostics are not provided.
- **Source-complete:** yes, for the command-local interface.

## csplit

- **Clause/source:** XCU `csplit` `ENVIRONMENT VARIABLES` lists `LANG`,
  `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and `NLSPATH`. Context
  operands use POSIX BREs, so XBD 9.3 class/equivalence/range rules apply.
- **Tests:** `TestCsplitLocaleRegexClassesEquivalenceAndRanges` and
  `TestCsplitLocaleInitFailsBeforeInputOpen`, plus the existing focused BRE,
  split, repeat, cleanup, suffix, and diagnostic tests.
- **Confirmed fix:** `csplit` now resolves LC_CTYPE and LC_COLLATE before input
  I/O and compiles context BREs through invocation-owned locale byte tables.
  The new tests cover non-C class membership, equivalence classes, collation
  ranges, provider close, and fail-before-open behavior.
- **Honest residual:** the shared ctype/collate providers are byte-oriented and
  bounded to C/POSIX plus reviewed Latin-1 aliases. A focused remaining red is
  `LC_ALL=C.UTF-8 csplit -s - '/[[:alpha:]]/'` with multibyte alphabetic input:
  fully conforming UTF-8 character-class and collation behavior needs a shared
  multibyte locale provider outside this issue's ownership.
- **Source-complete:** yes for the existing provider surface; no for arbitrary
  installed multibyte locales.

## expr

- **Clause/source:** XCU `expr` `ENVIRONMENT VARIABLES` lists `LANG`,
  `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and `NLSPATH`. String
  comparison uses LC_COLLATE; BRE matching uses LC_CTYPE and LC_COLLATE; string
  length, index, and substring are character operations.
- **Tests:** `TestExprLocaleCharacterBoundaries`,
  `TestExprLocaleRegexClassesEquivalenceAndRanges`,
  `TestExprLocaleCollationComparison`, plus the existing arithmetic, boolean,
  match, diagnostics, and EPIPE tests.
- **Confirmed fix:** `expr` now carries invocation-owned locale state through
  parsing. C/POSIX string functions count bytes as characters, C/POSIX UTF-8
  aliases use UTF-8 boundaries for string functions, Latin-1 LC_CTYPE uses the
  provider byte tables, LC_COLLATE comparisons call the invocation collator,
  and non-C BREs use the locale byte-regexp compiler.
- **Honest residual:** fully conforming multibyte LC_CTYPE regex class,
  equivalence, and collation behavior needs a shared multibyte provider beyond
  the current byte-oriented corpus. Back-reference support in the locale
  byte-regexp substrate is also absent for non-C BREs.
- **Source-complete:** yes for command-local C/POSIX, C/POSIX UTF-8 string
  boundaries, and the existing Latin-1 provider surface; no for full arbitrary
  multibyte regex locales.

## fold

- **Clause/source:** XCU `fold` `ENVIRONMENT VARIABLES` lists `LANG`,
  `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and `NLSPATH`; DESCRIPTION and OPTIONS
  require character boundaries, display columns, tabs, backspaces, carriage
  returns, bytes mode, and blank-sensitive breaks.
- **Tests:** `TestIssue735LocaleCharacterBoundariesPreserveOriginalBytes`,
  `TestIssue735MalformedAndCLocaleBytesAreNeverReencoded`,
  `TestIssue735SpacesAndControlCharactersAcrossLocales`,
  `TestIssue735LCCTypePrecedence`,
  `TestIssue735UnsupportedLocaleFailsBeforeInput`,
  `TestIssue735UnsupportedLocaleFailsBeforeOpeningOperand`,
  `TestIssue735ReadAndShortWriteErrors`, and
  `TestIssue735RunReportsReadAndShortWriteErrors`, plus the public fold tests.
- **Confirmed state:** fold already had command-local LC_CTYPE coverage for
  C/POSIX bytes, C/POSIX UTF-8 aliases, and the carried Latin-1 locale, while
  preserving original bytes and failing before I/O on unsupported locales.
- **Honest residual:** width tables and installed locale coverage are bounded;
  translated diagnostics are not provided.
- **Source-complete:** yes, for the command-local interface and carried locale
  corpus.

## Gate run for this packet

Owned-package gates, all green as of this packet:

- `gofmt -l cmds/awk cmds/comm cmds/csplit cmds/expr cmds/fold` — no output (clean).
- `go test -count=20 ./cmds/awk ./cmds/comm ./cmds/csplit ./cmds/expr ./cmds/fold` — ok.
- `go test -race -count=5 ./cmds/awk ./cmds/comm ./cmds/csplit ./cmds/expr ./cmds/fold` — ok.
- `go vet ./cmds/awk ./cmds/comm ./cmds/csplit ./cmds/expr ./cmds/fold` — clean.
- `GOOS={windows,linux,darwin} go vet` of the five owned packages — all PASS
  (the cross-OS legs `scripts/crossvet.sh` would run; verified directly here
  because that script exits early on the manifest check noted below).

### Conductor-owned residual: applet matrix refresh

`scripts/applet-matrix.py --check` (and therefore `scripts/crossvet.sh`, which
invokes it) currently report **stale**. The regenerator's diff is exactly two
mechanical test-function count cells in the generated manifests
`docs/applet-matrix.md` and `docs/applet-matrix.tsv`, both a direct function of
the owned test additions in this branch:

- `csplit`: `1 | 18` → `1 | 20`
- `expr`: `1 | 9` → `1 | 12`

No inventory, family, ownership, or coverage assertion changes; no package loses
tests. Those manifests are shared generated artifacts explicitly outside this
packet's ownership, and the refresh is historically a separate
maintainer-authored commit (for example `c354351` "docs: refresh applet matrix
for fold coverage", `056e48a` for iconv, `befd0dc` for expand — none authored by
the weave worker). This packet therefore does **not** edit them; the conductor
closes the gate with the standard step:

```sh
python3 scripts/applet-matrix.py   # rewrites docs/applet-matrix.{md,tsv}; then commit as "docs: refresh applet matrix for locale wave A"
```

After that refresh, `scripts/applet-matrix.py --check` and
`scripts/crossvet.sh` pass unchanged.

No generated manifest, consolidated report, shared script, sibling repository,
or unrelated package is edited by this closure packet.
