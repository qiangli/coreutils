# Locale wave B — issue 768: grep, join, sed, sort

Sprint 79 POSIX-only closure for the rank-2 non-C multibyte locale residual
named in [`sprint-79-consolidated.md`](sprint-79-consolidated.md) (row: *"Add
non-C multibyte `LC_CTYPE`/`LC_COLLATE`/`LC_NUMERIC` fixtures that discriminate
character boundaries, classes, equivalence, ranges, ordering, blanks, widths,
and numeric rendering for each named command"*). Normative authority is
POSIX.1-2016 Issue 7; GNU extensions are out of scope. The bounded certification
corpus is `C`/`POSIX` plus the single-byte `de_DE.ISO-8859-1` locale carried by
the invocation-owned providers `pkg/locale`, `pkg/ctype`, and `pkg/collate`. No
shared provider, generated manifest, or sibling repo was modified.

The providers reach glibc over dlopen and are built only for linux/amd64 and
linux/arm64; every other target returns `ErrUnsupportedPlatform`, so a non-C
locale on an unsupported host fails closed (exit 2), exactly as `sort` already
did. All executable evidence below therefore drives the invocation-owned seam
with a deterministic fake provider so the discriminating behavior is proven on
every platform, matching the established `cmds/sort/collator_test.go` and
`cmds/sed/ctype_test.go` patterns.

---

## grep — XCU:grep ENVIRONMENT_VARIABLES, STDOUT (LC_CTYPE, LC_COLLATE)

Clause/source: POSIX Issue 7 `grep` ENVIRONMENT VARIABLES (`LC_CTYPE` selects
character classes, case, and the interpretation of pattern/subject bytes as
characters; `LC_COLLATE` selects equivalence classes and collating elements in
the RE). grep's non-C support is pure-Go (`cmds/grep/locale.go`): the pattern is
decoded from ISO-8859-1 and augmented with the locale's class/equivalence
members, and the subject is decoded through `localeMatcher`, so it runs
identically on every platform without a glibc provider.

Confirmed Bashy-owned defects fixed (`cmds/grep/grep.go`, `cmds/grep/locale.go`):

1. **Raw high-byte pattern aborted with "invalid UTF-8."** A pattern containing
   an ISO-8859-1 high byte (e.g. `É` = 0xC9) was handed to RE2 undecoded, so
   `grep É`, `grep -i É`, and `grep -F É` under `LC_CTYPE=de_DE.ISO-8859-1`
   exited 2 with `error parsing regexp: invalid UTF-8` instead of matching the
   character. Fix: decode the pattern to runes (`latin1ToRunes`) whenever the
   invocation codeset is ISO-8859-1, for `-F` fixed strings as well as regular
   expressions, so RE2 compiles valid UTF-8 and the accented literal matches the
   decoded subject (case-sensitively, or folded under `-i`).
2. **The byte-oriented literal fast path silently lost every locale match.**
   Once the pattern was decoded to multi-byte UTF-8, the literal substring
   search compared decoded pattern bytes against raw single-byte input and
   matched nothing (a high-byte literal returned exit 1 / count 0). Fix: the
   literal fast path is disabled whenever the ISO-8859-1 decode is active; those
   runs stay on the RE2 + `localeMatcher` path that maps `-o` extents back to
   original byte offsets.
3. **German locale recognition ignored the codeset and the carried tables were
   incomplete.** Prefix matching accepted `de_DE.UTF-8`, Latin-9, and a bare
   `de_DE` as Latin-1, which could split valid UTF-8 output at a byte boundary.
   Locale selection now accepts only `C`, `POSIX`, and the two reviewed
   ISO-8859-1 aliases, with every other effective `LC_CTYPE`/`LC_COLLATE`
   failing with status 2 before pattern-file or operand input. The in-process
   table now covers all 256 bytes for every POSIX class, including isolated
   Latin-1 alphabetics, C1 controls, non-ASCII punctuation, graph, and print.
   The bounded German collation fixtures include the non-code-point ordering
   `a < ä < b` for ordinary and collating-element range forms.

Test IDs (`cmds/grep/locale_ctype_test.go`, new):

- `TestGrepLocaleCTypeDiscriminatesClassesAndCase` — literal high-byte match and
  case sensitivity (regression for defect 1/2), `-i` folding of `É/é` and `Ä/ä`,
  `[[:upper:]]`/`[[:lower:]]`/`[[:alpha:]]`/`[[:digit:]]` boundaries on the
  accented bytes, and `[[=a=]]` equivalence grouping the umlaut with its base.
- `TestGrepLocaleOnlyMatchingKeepsByteOffsets` — `-o` reports the original
  single-byte run, not its UTF-8 expansion.
- `TestGrepLocaleFixedStringHighByteIsByteExact` — `-F É` matches `É`
  byte-for-byte and stays case-sensitive.
- `TestGrepLocaleAllLatin1ClassMembership` — checks all 256 bytes against all
  twelve POSIX character classes.
- `TestGrepLocaleCompleteLatin1ClassesAndGermanRange` — pins isolated
  alphabetics, punctuation, C1 control/print distinctions, and both `[a-b]`
  range forms with `ä` between the endpoints.
- `TestGrepLocaleRejectsUnreviewedCodesetsBeforeInput` — UTF-8, Latin-9, and a
  codeset-less German locale fail closed instead of being decoded as Latin-1.

Pre-existing precedence coverage retained: `TestGrepVSCLocalePrecedence`,
`TestGrepPOSIXRegexConformance`.

Residual (out of this sprint's non-C scope, recorded not fixed): a raw high byte
in a `-F` pattern under the **`C`/`POSIX`** locale still aborts on RE2's
UTF-8 validation rather than matching the byte literally. That is a C-locale
fixed-string-vs-invalid-UTF-8 limitation, not a non-C multibyte `LC_CTYPE`
behavior, so it is left for a separate fixed-string-matcher change.

Source-complete eligibility: **eligible (implemented)** for the bounded
ISO-8859-1 `LC_CTYPE`/`LC_COLLATE` regex product. Integration verification is
manager/harness-owned.

---

## join — XCU:join ENVIRONMENT_VARIABLES, STDOUT (LC_COLLATE, LC_CTYPE)

Clause/source: POSIX Issue 7 `join` ENVIRONMENT VARIABLES — `LC_COLLATE`
determines the collating sequence used to order the input **and** the sequence
join uses to compare fields; `LC_CTYPE` determines byte→character
interpretation and which characters are blank.

Confirmed Bashy-owned defect fixed (`cmds/join/join.go`, `cmds/join/collate.go`
new): join compared join fields with `strings.Compare` (byte order) in every
locale, so under a non-C `LC_COLLATE` it disagreed with the order `sort`
produced and could both miss valid pairs and mis-diagnose sorted input. Fix:
field comparison now routes through the shared invocation-owned `pkg/collate`
provider when `LC_COLLATE` names a supported non-C locale, via a `collatorOpener`
seam resolved before any input file is opened (a missing provider or unsupported
locale fails closed with exit 2 before consuming operands). The same comparator
backs both join equality and the sorted-order check, so they stay consistent.
As GNU join does (keycmp uses `memcasecmp`, not `xmemcoll`, when `ignore_case`
is set), `-i` selects a case-insensitive comparison rather than a collating one
and stays off the collator; its case folding follows `LC_CTYPE` (see the second
fix below).

Second confirmed Bashy-owned defect fixed (`cmds/join/join.go`,
`cmds/join/ctype.go` new): `-i` folded ASCII case only, so under a non-C
`LC_CTYPE` the locale's high-byte letter pairs (`Ä`/`ä`) did not fold equal and
join failed to pair lines that GNU's `memcasecmp` pairs. Fix: when `-i` is set
and `LC_CTYPE` names a supported non-C locale, a 256-entry uppercase fold table
is snapshotted from the invocation-owned `pkg/ctype` provider (via a
`ctypeOpener` seam resolved before any input is opened, failing closed with
exit 2 on an unopenable/unsupported provider) and the provider is closed
immediately; `compareKeys` folds through that table instead of ASCII
`upperByte`. Folding to uppercase (not GNU's lowercase) keeps the fold
direction identical to the C-locale ASCII path; the load-bearing requirement —
case variants fold equal — holds either way.

Test IDs (`cmds/join/collate_test.go`, new):

- `TestJoinLCCollateOrdersFieldComparison` — an input ordered `a < ä < b` under
  de_DE collation (NOT byte order) still pairs the `ä` group with no disorder.
- `TestJoinLCCollateFlagsCollationDisorder` — byte-ascending `a, b, ä` is
  collation-disordered, so once an unpairable line is seen join reports
  `is not sorted` / `input is not in sorted order` and exits 1; the identical
  bytes under `C` are accepted with no collator opened.
- `TestJoinCPOSIXAndIgnoreCaseBypassCollator` — `C`, `POSIX`, and `-i` (even
  under a locale) never open a collator.
- `TestJoinLCCollateOpenFailurePrecedesInput` — provider open failure exits 2
  before the stdin operand is read.
- `TestJoinLCCollateCompareFailureIsDiagnosed` — a mid-run collation failure
  exits 1 with a diagnostic, closes the provider, and emits no tentatively
  paired output.
- `TestJoinLCCollateCloseFailureDiscardsOutput` — provider close failure exits
  1 and discards the staged Cartesian product.

Join stages the complete result invocation-locally. It commits the staged bytes
to standard output only after all comparisons and the collator close succeed;
comparison or close failures therefore cannot expose false equality products.

`-i` `LC_CTYPE` fold test IDs (`cmds/join/ctype_test.go`, new):

- `TestJoinLCCTypeFoldsHighByteForIgnoreCase` — `Ä`(0xC4) and `ä`(0xE4) pair
  under `-i` in a de_DE `LC_CTYPE`; the provider is opened once and closed after
  snapshot.
- `TestJoinLCCTypeCKeepsASCIIFold` — under `LC_CTYPE=C`, `-i` folds ASCII only
  (`A`/`a` pair, `Ä`/`ä` do not) and opens no provider.
- `TestJoinLCCTypeOpenFailureFailsClosed` — an unopenable provider under `-i`
  exits 2 before any operand is read.
- `TestJoinLCCTypeSnapshotFailure` — a `ToUpper` failure during snapshot exits 2
  and still closes the provider.
- `TestJoinWithoutIgnoreCaseNeverOpensCtype` — a plain byte-order run under a
  non-C `LC_CTYPE` never opens the provider.

Residual (recorded, not fixed): the blank class for default field splitting is
`LC_CTYPE`-defined; in the bounded `C`/de_DE.ISO-8859-1 corpus the blank class
is exactly space+tab, so no observable behavior is missing there.

Source-complete eligibility: **eligible (implemented)** for the bounded non-C
`LC_COLLATE` field-comparison and `-i` `LC_CTYPE` case-folding products.
Integration verification is manager/harness-owned.

---

## sed — XCU:sed ENVIRONMENT_VARIABLES, STDOUT (LC_CTYPE, LC_COLLATE)

Clause/source: POSIX Issue 7 `sed` ENVIRONMENT VARIABLES (`LC_CTYPE` character
classes, case folding for the `I` flag and `y`, high-byte character boundaries;
`LC_COLLATE` equivalence/collating bracket members). No code change: sed already
resolves `LC_CTYPE` and `LC_COLLATE` independently through invocation-owned
`ctypeOpener`/`collateOpener` seams (`cmds/sed/sed.go`
`runCommandWithLocales`), fails closed before operand I/O, and closes providers
on every path.

Existing test IDs verified adequate for the bounded product
(`cmds/sed/ctype_test.go`): `TestSedCTypeResolverLifecycleAndBypass` (class
membership over all 256 bytes, high-byte addresses/patterns in BRE and ERE, `I`
case folding of high bytes, raw high-byte delimiters/replacements),
`TestSedResolvesCTypeAndCollateIndependently` (LC_CTYPE and LC_COLLATE resolved
from separate categories), `TestSedLocaleEquivalenceClassMatchesInBothGrammars`
(`[[=a=]]`/`[[.a.]]` in BRE and ERE), `TestSedCTypeErrorsCloseAndPrecedeIO`,
`TestSedLocaleCompileFailsBeforeOperandIO`,
`TestSedCTypeConcurrentInvocationEnvironments`. sed does not consult
`LC_NUMERIC`.

`TestSedLocaleCollationRangeMatchesInBothGrammars` supplies nonidentity
collation weights with `a < ä < b` and proves at the command surface that both
BRE and ERE `[a-b]` match the raw Latin-1 umlaut. This distinguishes locale
range handling from byte/code-point-order fallback.

Fix/residual: no sed product-source change was owed for the bounded corpus.
`TestSedLocaleCollationRangeMatchesInBothGrammars` was added to make the
nonidentity `LC_COLLATE` range product explicit in both BRE and ERE without
locking limitations or imposing synthetic catalogs.

Source-complete eligibility: **eligible (implemented)** for the bounded non-C
`LC_CTYPE`/`LC_COLLATE` product. Integration verification is
manager/harness-owned.

---

## sort — XCU:sort ENVIRONMENT_VARIABLES, STDOUT (LC_COLLATE, LC_NUMERIC, LC_CTYPE)

Clause/source: POSIX Issue 7 `sort` ENVIRONMENT VARIABLES (`LC_COLLATE`
collating order; `LC_NUMERIC` radix/thousands for `-n`; `LC_CTYPE` case for
`-f` and character classes for `-d`/`-i`). `LC_COLLATE` and `LC_NUMERIC` were
already implemented and discriminatingly tested; this sprint closes the
`LC_CTYPE` half.

`LC_COLLATE`/`LC_NUMERIC` test IDs verified adequate (unchanged):

- `LC_COLLATE` ordering — `cmds/sort/collator_test.go`:
  `TestRunWithCollatorOrdersTextualComparisons`, `TestRunWithCollatorMerge`,
  `TestRunWithCollatorCompareFailureDiscardsOutput`,
  `TestRunWithCollatorInitFailureHasNoInputRandomFiles0OrOutputSideEffects`,
  `TestRunWithCollatorKeepsCAndPOSIXByteOrdered` (fake collator drives ordering,
  merge, check, unique, failure-discard, init-before-I/O, and C/POSIX bypass).
- `LC_NUMERIC` rendering — `cmds/sort/sort_test.go` `TestSortLCNumeric`
  (de_DE comma decimal and dot thousands under `-n`, plus LC_ALL/LC_NUMERIC/LANG
  precedence).

Confirmed Bashy-owned defect fixed (`cmds/sort/sort.go`, `cmds/sort/ctype.go`
new): `-f`/`-d`/`-i` normalized keys with ASCII-only byte tables
(`normalizeTextKey` in `cmds/sort/compare.go`), so under a non-C `LC_CTYPE`
`-f` did not fold `Ä/ä` to equal and `-d`/`-i` classified only ASCII. Fix: when
an active key uses `-f`/`-d`/`-i` and `LC_CTYPE` names a supported non-C locale,
sort snapshots 256-entry `fold`/`IsAlnum`/`IsBlank`/`IsPrint` tables from the
invocation-owned `pkg/ctype` provider (via a `ctypeOpener` seam resolved after
key validation, failing closed with exit 2 on an unopenable/unsupported
provider) and closes the provider immediately; `normalizeTextKey` then routes
through the immutable snapshot so the hot comparison path stays table-driven
(the `cmds/sed` shape). C/POSIX and any key that uses none of `-f`/`-d`/`-i`
never open the provider. Folding to uppercase matches the prior ASCII direction.

Test IDs (`cmds/sort/ctype_test.go`, new):

- `TestSortLCCTypeFoldsHighByteLetters` — `-f -u` folds `Ä/ä` equal and collapses
  the pair.
- `TestSortLCCTypeFoldOrdersAcrossCase` — the folded key drives order, not just
  equality.
- `TestSortLCCTypeDictKeepsHighByteAlnum` — `-d` keeps the locale's high-byte
  alphanumerics while dropping punctuation.
- `TestSortLCCTypeIgnoreNPDropsNonPrint` — `-i` drops a locale-non-printing C1
  byte and treats the remaining keys as equal.
- `TestSortLCCTypeUnsupportedFailsClosed` / `TestSortLCCTypeSnapshotFailurePrecedesOutput`
  — open and snapshot failures exit 2 with no output, closing the provider.
- `TestSortCPOSIXNeverOpensCtype` / `TestSortNoTextKeyNeverOpensCtype` — C/POSIX
  and non-classifying runs never open the provider (ASCII fold retained).

Residual (recorded, not fixed): none for the bounded `C`/de_DE.ISO-8859-1
corpus.

Source-complete eligibility: **eligible (implemented)** for the mandatory non-C
`LC_COLLATE`, `LC_NUMERIC`, and `-f`/`-d`/`-i` `LC_CTYPE` products. Integration
verification is manager/harness-owned.

---

## Gate

`gofmt -l`; `go test -count=20` and `go test -race -count=5` for each of
`cmds/grep`, `cmds/join`, `cmds/sed`, `cmds/sort`; `go vet`;
`git diff --check`.

Generated applet-matrix projections are manager-owned and intentionally not
regenerated by this issue; its cumulative diff contains no projection change.
