# POSIX interface audit — Go batch 3 (issue 28)

This document rewrites the audit previously recorded in run-local commit
`af2cd12`, which was **not merged as-is**. The dispositions below come from an
independent review and are grounded in the current tree. It audits the
thirteen Go-availability commands assigned to this batch against
POSIX.1-2016 (Issue 7, 2016 Edition) using exact 2016 edition URLs.

## Ground rules

- Conformance is judged against the official POSIX.1-2016 utility pages only;
  GNU compatibility remains out of scope for certification per
  `docs/reference-policy.md`.
- Evidence references are stable `file#TestID` pairs pointing at existing
  tests in this tree; source anchors are `file:line` snapshots of the audited
  revision.
- **No ledger promotion.** This audit changes no evidence state in
  `docs/posix-required-command-interfaces.tsv` or
  `docs/posix-required-command-interfaces.md`. All thirteen rows remain
  `unverified`. Promotion requires separate runs after the required
  corrections land, each backed by focused behavioral evidence.

## Disposition summary

| Command | Disposition | Core finding |
| --- | --- | --- |
| fold | implementation/evidence gap | corrupts non-UTF-8 public-locale input |
| getconf | implementation/evidence gap | missing much of the mandatory Base variable/name surface |
| grep | implementation/evidence gap | single hardcoded locale; byte BRE/ERE unrouted |
| iconv | implementation/evidence gap | partial: `-c` refused, omitted `-f`/`-t` unresolved |
| id | implementation/evidence gap | default output fails real/effective reporting |
| join | implementation/evidence gap | byte-only comparison; no locale |
| locale | implementation/evidence gap | cannot report advertised public non-C keyword data |
| logger | implementation/evidence gap | POSIX stdin unused semantics; Close errors swallowed |
| logname | implementation/evidence gap | falls back to effective user instead of failing |
| ls | implementation/evidence gap | Base `-g`/`-o` (XSI) ignore `LC_COLLATE`/`LC_CTYPE`/`LC_TIME` |
| head | supportable pass | Base surface supportable; `-c` is GNU semantics, not XSI |
| ln | supportable pass | Base `-f`/`-i` and XSI `-s` covered |
| mesg | supportable pass | query/set/exit-status contract covered |

Ten implementation/evidence gaps, three supportable passes. No command in
this batch is promoted.

---

## fold — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html
Ledger row: `unverified` — unchanged.

The implementation decodes input as UTF-8 runes and measures display width
with `runewidth.RuneWidth` (`cmds/fold/fold.go:138,159,241`). Input in an
8-bit public-locale charset is not decoded through that locale's charmap, so
non-UTF-8 bytes do not round-trip: the folded output is corrupted for any
non-UTF-8 public locale. Current tests cover UTF-8 fixtures only.

Evidence:
- `cmds/fold/fold_test.go#TestFoldDefaultUsesDisplayColumns`
- `cmds/fold/fold_test.go#TestFoldCharactersKeepsUTF8RunesWhole`
- `cmds/fold/fold_test.go#TestFoldBytesPreservesUtf8UnitsAtSmallWidth`

Required correction: decode input per the active locale's charmap (or treat
un-decodable bytes as single-width units that pass through untouched), and add
non-UTF-8 public-locale fixtures proving byte fidelity.

## getconf — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html
Ledger row: `unverified` — unchanged.

The queryable surface is `sysVars` plus `pathVars`
(`cmds/getconf/vars.go:28,77`; `cmds/getconf/sysconf_unix.go` is 64 lines).
Much of the mandatory Base variable/name surface — sysconf variables, pathconf
variables, confstr names, and `<limits.h>` minimum names — is not queryable.
What exists agrees with the host, but the standard's mandatory set is far from
covered, and per the ledger's fail-closed policy the missing mandatory names
cannot be claimed as verified.

Evidence:
- `cmds/getconf/getconf_test.go#TestAgreesWithSystemGetconf`
- `cmds/getconf/getconf_test.go#TestPathconfAgreesWithSystem`
- `cmds/getconf/getconf_test.go#TestCompileTimeMinimumsComeFromTheStandard`
- `cmds/getconf/getconf_test.go#TestUnknownVariableIsAnErrorNotUndefined`
- `cmds/getconf/getconf_test.go#TestAllListsAndDoesNotTakeOperands`

Required correction: enumerate the complete mandatory Base set from the
standard's tables, implement each name, and keep loud refusal only for
genuinely unsupported names.

## grep — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/grep.html
Ledger row: `unverified` — unchanged.

Locale handling is a single hardcoded certification locale: the tables in
`cmds/grep/locale.go` exist only for the repository's de_DE ISO-8859-1 locale,
and environment lookup collapses `LC_ALL`/category/`LANG` to that one case.
Other public locales are not honored. Separately, BRE/ERE matching is not
routed through a byte-charset path: matching in non-UTF-8 (byte) locales is
unrouted.

Evidence:
- `cmds/grep/grep_test.go#TestGrepVSCLocalePrecedence`
- `cmds/grep/grep_test.go#TestGrepPOSIXRegexConformance`
- `cmds/grep/grep_test.go#TestGrepBREBackrefConformance`
- `cmds/grep/grep_test.go#TestGrepREDUPMAX2047Intervals`
- `cmds/grep/literal_test.go#TestLiteralFastPathDifferential`

Required correction: route locale selection through the runtime locale
facility rather than the hardcoded German tables, and route BRE/ERE matching
through charset-aware paths for byte locales.

## iconv — gap (remains partial)

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html
Ledger row: `unverified` — unchanged.

Two documented behaviors remain unimplemented: `-c` (omit invalid characters)
is refused loudly — `cmds/iconv/iconv.go:92` registers `discard-invalid`, and
the test asserts refusal — and omitted `-f`/`-t` do not resolve to the
documented default codeset; a missing encoding is a usage error instead. The
converter itself round-trips its supported encodings, but the option surface
stays partial.

Evidence:
- `cmds/iconv/iconv_test.go#TestUnsupportedEncodingAndDiscardOptionFailLoudly`
- `cmds/iconv/iconv_test.go#TestMissingEncodingIsUsageError`
- `cmds/iconv/iconv_test.go#TestUTF8ToISO88591`
- `cmds/iconv/iconv_test.go#TestGB18030RoundTrip`
- `cmds/iconv/iconv_test.go#TestAllSupportedEncodingsAreResolvable`

Required correction: implement `-c` discard semantics and the omitted
`-f`/`-t` default resolution, or keep both as documented loud refusals while
the ledger stays partial — never approximate silently.

## id — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html
Ledger row: `unverified` — unchanged.

The default output fails real/effective reporting. The flags document only the
effective identity (`cmds/id/id.go:30-35`: `-u`/`-g` "effective",
`-r` "real instead of effective"), and the default format cannot express the
real/effective distinction at all — `-r` and name-only output are rejected in
default format (`cmds/id/id.go:58-61`). The standard's default report of real
versus effective identity is therefore not produced.

Evidence:
- `cmds/id/id_test.go#TestIDDefault`
- `cmds/id/id_test.go#TestIDDefaultIncludesNames`
- `cmds/id/id_test.go#TestIDRealFlag`
- `cmds/id/id_unix_test.go#TestIDRealAndEffectiveSelectors`

Required correction: default output must report the real/effective identity
distinction the standard's format requires when they differ; `-r` must be
expressible alongside the default format.

## join — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html
Ledger row: `unverified` — unchanged.

Key comparison is C-locale byte order only — `cmds/join/join.go:12` states it,
and `compareKeys` reduces to `strings.Compare` (`cmds/join/join.go:513-515`).
There is no locale (collation) dimension: join-field ordering and the
"input not sorted" checks ignore the locale entirely.

Evidence:
- `cmds/join/join_test.go#TestJoin`
- `cmds/join/join_test.go#TestJoinStdin`
- `cmds/join/join_test.go#TestJoinOrderCheck`

Required correction: route key comparison and order checks through locale
collation per the standard, with byte order only for the C locale.

## locale — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html
Ledger row: `unverified` — unchanged.

The utility cannot report advertised public non-C keyword data. Keyword values
come from the repository's built-in tables (German ISO-8859-1 time data is the
only non-C dataset); every other public locale is refused by name rather than
read from a locale archive. `writeNames`
(`cmds/locale/locale.go:127`) therefore answers from a fixed inventory, not
from the public locale database.

Evidence:
- `cmds/locale/locale_test.go#TestKeywordValues`
- `cmds/locale/locale_test.go#TestGermanISO88591TimeData`
- `cmds/locale/locale_test.go#TestUnavailableLocaleIsRefusedByName`
- `cmds/locale/locale_test.go#TestCategoryOperandWritesEveryKeyword`

Required correction: source keyword data from the public locale archive for
advertised locales, so `locale <keyword>` reports real public data instead of
refusing every non-built-in name.

## logger — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html
Ledger row: `unverified` — unchanged.

Two corrections are required. First, POSIX stdin is unused by this utility —
the synopsis passes message string operands — yet the implementation reads
stdin when operands are omitted; that behavior is a GNU extension and must not
be presented as the POSIX surface. Second, Close errors matter: the sink
interface has `Close() error` (`cmds/logger/logger.go:51`) but it is invoked
only via `defer s.Close()` (`cmds/logger/logger.go:99`), so finalization
failures are silently dropped rather than reported in the exit status.

Evidence:
- `cmds/logger/logger_test.go#TestNoOperandsReadsStdinOneRecordPerLine`
- `cmds/logger/logger_test.go#TestStdinWithoutTrailingNewlineStillLogsTheLastLine`
- `cmds/logger/logger_test.go#TestEmptyStdinLogsNothingAndSucceeds`
- `cmds/logger/logger_test.go#TestSinkOpenFailureIsReported`
- `cmds/logger/logger_test.go#TestSendFailureIsReported`

Required correction: label or gate the stdin path as a GNU extension so the
POSIX operand form is the certified surface, and propagate sink `Close()`
errors into the exit status.

## logname — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html
Ledger row: `unverified` — unchanged.

When no login name is available, logname must fail. Instead of failing, the
implementation falls back from the login-uid lookup to a pure-Go effective
user lookup (`cmds/logname/logname.go:46-54`), reporting the effective account
as the login name. The environment-account names are correctly ignored, but
the fallback defeats the failure contract.

Evidence:
- `cmds/logname/logname_test.go#TestLogname`
- `cmds/logname/logname_test.go#TestLognameIgnoresEnvironmentAccountNames`
- `cmds/logname/logname_test.go#TestLognameNoLoginName`
- `cmds/logname/logname_test.go#TestResolveLoginUID`
- `cmds/logname/logname_test.go#TestLoginNameFromLoginUIDEmptyOffLinux`

Required correction: remove the effective-user fallback; exit non-zero with
the diagnostic when the getlogin-equivalent lookup yields nothing.

## ls — gap

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html
Ledger row: `unverified` — unchanged.

The Base disposition with the XSI `-g`/`-o` long-format options
(`cmds/ls/ls.go:187-188`) ignores locale categories entirely: there is no
`LC_COLLATE`, `LC_CTYPE`, or `LC_TIME` handling anywhere in the implementation.
Sort order is not collation-aware, control-character classification for
display formatting is not ctype-aware, and long-format timestamps do not
follow `LC_TIME`.

Evidence:
- `cmds/ls/ls_test.go#TestDefaultSortAndOnePerLine`
- `cmds/ls/ls_posix_test.go#TestHideControlChars`
- `cmds/ls/ls_posix_test.go#TestOrderLongAndOneFormat`
- `cmds/ls/ls_test.go#TestLongFormat`

Required correction: honor `LC_COLLATE` for ordering, `LC_CTYPE` for character
classification, and `LC_TIME` for time formats across the formatted outputs
(including the `-g`/`-o` forms).

## head — supportable pass

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/head.html
Ledger row: `unverified` — unchanged (no promotion from this audit).

The Base surface (`-n number`, operands, persistent headers on multiple
files, obsolete header option forms) is implemented and tested. One caveat is
recorded: the implemented `-c` is GNU semantics — leading `-` meaning "all but
the last NUM", unit suffixes, `--bytes` alias
(`cmds/head/head.go:33`) — not the XSI `-c` byte-count option. Since GNU
compatibility is out of scope, the Base surface is supportable and the XSI
`-c` option is simply not certified.

Evidence:
- `cmds/head/head_test.go#TestHead`
- `cmds/head/head_test.go#TestHeadHeaders`
- `cmds/head/head_test.go#TestParseCount`
- `cmds/head/head_test.go#TestHeadAbbreviatedHeaderOptionsRespectOrder`
- `cmds/head/head_test.go#TestHeadObsoleteHeaderOptionsRespectOrder`

## ln — supportable pass

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html
Ledger row: `unverified` — unchanged (no promotion from this audit).

Base `-f` and `-i` plus the XSI `-s` symbolic-link form are implemented and
exercised, including operand arity, destination-directory behavior, same-file
diagnostics, and error paths. GNU extras (backup, suffix, target-directory,
verbose) exist as clearly separate extensions.

Evidence:
- `cmds/ln/ln_test.go#TestLnHard`
- `cmds/ln/ln_test.go#TestLnSymbolic`
- `cmds/ln/ln_test.go#TestLnForce`
- `cmds/ln/ln_test.go#TestLnInteractiveAcceptsAndDeclines`
- `cmds/ln/ln_test.go#TestLnSingleOperand`
- `cmds/ln/ln_test.go#TestLnIntoDirectory`
- `cmds/ln/ln_test.go#TestLnErrors`
- `cmds/ln/ln_test.go#TestLnSameFile`

## mesg — supportable pass

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html
Ledger row: `unverified` — unchanged (no promotion from this audit).

The contract is covered: query reports terminal permission through the exit
status, setting toggles exactly the group-write bit, and operand/usage errors
are rejected loudly.

Evidence:
- `cmds/mesg/mesg_test.go#TestQueryReportsStateThroughExitStatus`
- `cmds/mesg/mesg_test.go#TestSetTogglesOnlyTheGroupWriteBit`
- `cmds/mesg/mesg_test.go#TestBadOperandAndExtraOperand`

---

## Reproducing the evidence

```sh
go test ./cmds/fold ./cmds/getconf ./cmds/grep ./cmds/iconv ./cmds/id \
  ./cmds/join ./cmds/locale ./cmds/logger ./cmds/logname ./cmds/ls \
  ./cmds/head ./cmds/ln ./cmds/mesg
```

## Ledger impact

None. `docs/posix-required-command-interfaces.tsv` and its generated markdown
are untouched by this audit; all thirteen rows remain `unverified`. The three
supportable passes (head, ln, mesg) are prerequisites for future promotion
runs, not promotions themselves.
