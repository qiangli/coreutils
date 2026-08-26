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

Test IDs (`cmds/grep/locale_ctype_test.go`, new):

- `TestGrepLocaleCTypeDiscriminatesClassesAndCase` — literal high-byte match and
  case sensitivity (regression for defect 1/2), `-i` folding of `É/é` and `Ä/ä`,
  `[[:upper:]]`/`[[:lower:]]`/`[[:alpha:]]`/`[[:digit:]]` boundaries on the
  accented bytes, and `[[=a=]]` equivalence grouping the umlaut with its base.
- `TestGrepLocaleOnlyMatchingKeepsByteOffsets` — `-o` reports the original
  single-byte run, not its UTF-8 expansion.
- `TestGrepLocaleFixedStringHighByteIsByteExact` — `-F É` matches `É`
  byte-for-byte and stays case-sensitive.

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
and stays on the byte path.

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
  exits 1 with a diagnostic and closes the provider rather than emitting a
  silently misordered result.

Residuals (recorded, not fixed): (a) `-i` folds ASCII case only; GNU's
`memcasecmp` folds per `LC_CTYPE`, so `Ä/ä` do not fold under `-i` in a de_DE
locale — closing this needs an `LC_CTYPE` `ToLower` seam (`pkg/ctype`,
snapshot-table shape) inside `cmds/join`, no shared-provider change. (b) The
blank class for default field splitting is `LC_CTYPE`-defined; in the bounded
`C`/de_DE.ISO-8859-1 corpus the blank class is exactly space+tab, so no
observable behavior is missing there.

Source-complete eligibility: **eligible (implemented)** for the bounded non-C
`LC_COLLATE` field-comparison product; `-i` locale case-folding is an explicit
residual above. Integration verification is manager/harness-owned.

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

Fix/residual: none owed for the bounded corpus. No new test added because the
discriminating multibyte `LC_CTYPE`/`LC_COLLATE` product is already covered and
the sprint prohibits locking limitations or imposing synthetic catalogs.

Source-complete eligibility: **eligible (implemented)** for the bounded non-C
`LC_CTYPE`/`LC_COLLATE` product. Integration verification is
manager/harness-owned.

---

## sort — XCU:sort ENVIRONMENT_VARIABLES, STDOUT (LC_COLLATE, LC_NUMERIC, LC_CTYPE)

Clause/source: POSIX Issue 7 `sort` ENVIRONMENT VARIABLES (`LC_COLLATE`
collating order; `LC_NUMERIC` radix/thousands for `-n`; `LC_CTYPE` case for
`-f` and character classes for `-d`/`-i`). No code change: the two mandatory
non-C products are already implemented and discriminatingly tested.

Existing test IDs verified adequate:

- `LC_COLLATE` ordering — `cmds/sort/collator_test.go`:
  `TestRunWithCollatorOrdersTextualComparisons`, `TestRunWithCollatorMerge`,
  `TestRunWithCollatorCompareFailureDiscardsOutput`,
  `TestRunWithCollatorInitFailureHasNoInputRandomFiles0OrOutputSideEffects`,
  `TestRunWithCollatorKeepsCAndPOSIXByteOrdered` (fake collator drives ordering,
  merge, check, unique, failure-discard, init-before-I/O, and C/POSIX bypass).
- `LC_NUMERIC` rendering — `cmds/sort/sort_test.go` `TestSortLCNumeric`
  (de_DE comma decimal and dot thousands under `-n`, plus LC_ALL/LC_NUMERIC/LANG
  precedence).

Residual (recorded, not fixed): `-f`/`-d`/`-i` normalize keys with ASCII-only
byte tables (`normalizeTextKey` in `cmds/sort/compare.go`), so under a non-C
`LC_CTYPE` `-f` does not fold `Ä/ä` to equal and `-d`/`-i` classify only ASCII.
Closing this needs an `LC_CTYPE` provider seam inside `cmds/sort` that snapshots
256-entry fold/`IsAlnum`/`IsBlank`/`IsPrint` tables at init (the `cmds/sed`
shape) so the hot comparison path stays table-driven; it is a `cmds/sort`-local
change, not a shared-provider change. No limitation-locking test is added, per
the sprint's prohibition. The observable effect is confined to `-f`-equality and
`-d`/`-i` filtering of high-byte letters; `LC_COLLATE` ordering already places
case/umlaut variants adjacently.

Source-complete eligibility: **eligible (implemented)** for the mandatory non-C
`LC_COLLATE` and `LC_NUMERIC` products; the `-f`/`-d`/`-i` `LC_CTYPE` refinement
is an explicit residual above. Integration verification is manager/harness-owned.

---

## Gate

`gofmt -l`; `go test -count=20` and `go test -race -count=5` for each of
`cmds/grep`, `cmds/join`, `cmds/sed`, `cmds/sort`; `go vet`;
`scripts/applet-matrix.py --check`; `scripts/crossvet.sh`.
