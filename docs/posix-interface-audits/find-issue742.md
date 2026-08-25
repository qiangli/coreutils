# find — Issue 7 interface closure (issue 742, Sprint 79)

Scope: `cmds/find` source and tests only. Normative source: POSIX.1-2016
XCU:find — https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html
(conformance controlled per docs/reference-policy.md; GNU behavior is an
extension lane, not the bar). This note maps every Issue 7
primary/action/operator named in the shared ledger row to focused
behavioral evidence and records the one normative fix plus the added
products of this run. The shared ledger/matrix/consolidated docs are not
edited here; a recommendation for them is at the end.

## Normative fix: path operands are required in POSIX mode

The Issue 7 synopsis is `find [-H|-L] path... [expression]`: one or more
path operands are required, and the ledger's operand_rules column records
that Bashy's no-path default of `.` "is an upstream extension, not POSIX
evidence." Until this run the extension was unconditional — a
no-argument or expression-first invocation silently searched `.`.

`find.go` now gates on POSIXLY_CORRECT (presence alone, even to an empty
value, matching every other gate in this repo):

- no operands at all → usage error, `missing path operand …`;
- an expression-first token (`-name …`, `(... )`, `! …`) → usage error
  naming the offending token, `paths must precede expression: '<tok>'`;
- `--` alone consumes the options but is not a path operand → the
  missing-path diagnostic;
- with at least one path operand the same environment behaves normally
  (the gate changes operand requirements only).

Outside POSIX mode the documented `.` default stands, preserved as an
extension. Evidence: `cmds/find/issue742_test.go#TestFindIssue7POSIXModeRequiresPathOperand`
(both `POSIXLY_CORRECT=1` and `POSIXLY_CORRECT=`), with the paired
control `#TestFindIssue7NoPathDefaultIsExtensionOutsidePOSIXMode`.

## Required primaries / actions / operators → evidence

| Ledger element | Focused evidence (all in `cmds/find/`) |
| --- | --- |
| `path` operands (SYNOPSIS/OPERANDS) | `issue742_test.go#TestFindIssue7POSIXModeRequiresPathOperand`, `#TestFindIssue7NoPathDefaultIsExtensionOutsidePOSIXMode`; multi-operand order `find_test.go#TestFindTests`; operand spelling `find_test.go#TestFindGeneralPathnameResolution` |
| `-name pattern` | `issue7_test.go#TestFindIssue7NameLeadingPeriodNotSpecial`; `find_test.go#TestFindTests`; operand basename `conformance_test.go#TestFindNameMatchesOperandBaseName`; C-locale bytes `conformance_test.go#TestFindPatternsAreCLocaleAndLocaleInvariant`, `#TestFindINameFoldsASCIIOnly`, `#TestFnmatch` |
| `-path pattern` | `find_test.go#TestFindTests` (`-path *deep*`, `-path ./sub/*`); `'*'` crossing `/` in `#TestFnmatch` |
| `-nouser` | seam: `issue742_unix_test.go#TestFindIssue7OwnershipSeamUnassignedOwnerAndGroup` (dynamically chosen unassigned id, never skipped on ordinary hosts); stub `issue7_unix_test.go#TestFindIssue7NouserUnownedPositivePath`; negative `find_test.go#TestFindUserGroup`; privileged real file `issue742_unix_test.go#TestFindIssue7RealOwnershipSeamUnassignedOwner` |
| `-nogroup` | same seam test (unassigned-gid and both-unassigned cases, non-skipped); negative `issue742_unix_test.go#TestFindIssue7NamedGroupOperand`; privileged counterpart above |
| `-user uname` | name and numeric id `find_test.go#TestFindUserGroup`; numeric-fallback-when-no-name `issue742_unix_test.go#TestFindIssue7NumericUserAndGroupOperandsUseTheIDWhenNoNameExists`; unknown name `find_test.go#TestFindErrors` |
| `-group gname` | named group `issue742_unix_test.go#TestFindIssue7NamedGroupOperand`; numeric `find_test.go#TestFindUserGroup` |
| `-xdev` | `find_test.go#TestFindXdev` (same-fs walk identity; loud not-supported on platforms without device identity) |
| `-prune` | `find_test.go#TestFindTests` (`-name skipme -prune -o … -print`) |
| `-perm [-]mode / [-]onum` | `find_test.go#TestFindPerm`; full symbolic grammar `conformance_test.go#TestParsePermBitsSymbolic`, `#TestFindPermSymbolic` (permcopy, `X`, `-`/`/` prefixes) |
| `-type c` | `find_test.go#TestFindTests`, `#TestFindTypeSpecialFiles` (b c p s via mode seam), `#TestFindTypeSymlink` |
| `-links n` | `find_test.go#TestFindLinks` (hard link, nlink 1); trichotomy `issue7_test.go#TestFindIssue7NumericArgumentTrichotomy` |
| `-size n[c]` | `find_test.go#TestFindTests`; 512-block round-up + trichotomy `issue7_test.go#TestFindIssue7NumericArgumentTrichotomy` |
| `-atime n` / `-ctime n` | `find_test.go#TestFindAtimeCtime` (linux/darwin stat seam) |
| `-mtime n` | `find_test.go#TestFindMtimeAndNewer` |
| `-newer file` | positive/negative `find_test.go#TestFindMtimeAndNewer`; missing-reference diagnostic+status `issue742_unix_test.go#TestFindIssue7NewerMissingReference` |
| `-exec utility_name [argument...] ;` | argv fidelity `exec_test.go#TestFindExecSemicolon`, `#TestFindExecArgvInjectionSafe`; child status is the primary's truth `#TestFindExecStatusSemantics`; argv[0] convention `#TestFindExecChildArgvZeroIsNameAsGiven`; not-found `#TestFindExecCommandNotFound`; ENOEXEC retry `exec_enoexec_unix_test.go` (shebangless family); signal mapping `exec_signal_unix_test.go#TestRunArgvSignaledExitCode` |
| `-exec ... {} +` | batching + non-zero ⇒ find exits non-zero `exec_test.go#TestFindExecPlus`; strict grammar `#TestFindExecPlusGrammar`; side effects and per-batch invocation count `issue742_test.go#TestFindIssue7ExecSideEffectsAndBatching` |
| `-ok utility_name [argument...] ;` | prompt/decline/EOF `exec_test.go#TestFindOk`; yesexpr anchor `#TestFindOkAffirmationAnchoredAtLineStart`; LC_MESSAGES/LC_ALL precedence end-to-end `issue742_test.go#TestFindIssue7LCAllPrecedenceForOKAffirmative` |
| `-print` | default action `find_test.go#TestFindDefaultPrintLexical`; `-print0` `#TestFindPrint0`; with explicit operand prefix `#TestFindTests` |
| `-depth` | `find_test.go#TestFindDepthOrder` |
| `( expression )` | `issue7_test.go#TestFindIssue7OperatorPrecedence`; `find_test.go#TestFindTests`; unmatched-paren diagnostic `#TestFindErrors` |
| `! expression` | `issue7_test.go#TestFindIssue7OperatorPrecedence`; `find_test.go#TestFindTests` |
| implicit `-a` / explicit `-a` / `-o` (precedence `!` > `-a` > `-o`) | `issue7_test.go#TestFindIssue7OperatorPrecedence` (discriminating fixture) |
| `+n` / `n` / `-n` trichotomy | `issue7_test.go#TestFindIssue7NumericArgumentTrichotomy` |
| `-H` / `-L` option position only | `issue7_test.go#TestFindIssue7FollowOptionsOnlyLeading`; follow modes `find_test.go#TestFindSymlinkFollow`; loop detection `#TestFindSymlinkLoop` |
| `--` ends leading options | `issue7_test.go#TestFindIssue7DoubleDashEndsLeadingOptions`; `conformance_test.go#TestFindEndOfLeadingOptions` |

## Clause-level products added this run

| Clause | Product |
| --- | --- |
| ENVIRONMENT_VARIABLES (precedence LC_ALL > LC_* > LANG) | patterns through a real walk: `issue742_test.go#TestFindIssue7LCAllPrecedenceForPatterns` (runs where a raw Latin-1 0xe4 filename is spellable — Linux/ext4; skips on Darwin/APFS with EILSEQ, where the matcher seam is still pinned on every platform by `find_test.go#TestFindVSCLocalePrecedence` and `#TestFindGermanLocaleCategories`); `-ok` affirmative: `#TestFindIssue7LCAllPrecedenceForOKAffirmative` |
| STDIN (`-ok` replies only) | `exec_test.go#TestFindOk`, `#TestFindOkAffirmationAnchoredAtLineStart` |
| EFFECTS (`-exec`/`-ok` side effects; batching) | `issue742_test.go#TestFindIssue7ExecSideEffectsAndBatching` — children append to one log file; `;` form = one MARK per path, `{} +` form = exactly one invocation for the batch |
| CONSEQUENCES_OF_ERRORS / STDERR (traversal, read errors) | `issue742_unix_test.go#TestFindIssue7UnreadableDirectoryTraversal` (unreadable directory and unreadable start point: diagnosed, walk continues, exit 1); missing start point `find_test.go#TestFindErrors`, `issue742_test.go#TestFindIssue7StatusAggregation` |
| STDOUT write failures | `conformance_test.go#TestFindWriteErrorIsDiagnosed` (one diagnostic, exit 1, `-print0` too) |
| EXIT_STATUS (aggregation) | `issue742_test.go#TestFindIssue7StatusAggregation`: bad start point either side of a good operand (good operand still reports, exit 1); all-good operands exit 0; declined `-ok` exits 0; failing `-exec {} +` exits 1 with paths still printed |

## Ownership seam determinism

Fixed numeric ids can resolve to names on some hosts, voiding a
positive `-nouser`/`-nogroup` fixture. `unassignedID` (in
`issue742_unix_test.go`) probes the real lookup for a free id in the
60000–65534 range per host, so the seam product is deterministic
everywhere it can run at all; the privileged real-chown fixture is
supplemental (root-only) evidence of the same behavior end-to-end.

## Known boundaries (unchanged, outside this run's scope)

- `-H`/`-L` symlink follow and `-xdev` are loud not-supported errors on
  platforms without the stat identity fields (windows) — fail-loudly
  contract, not a silent approximation.
- The locale corpus is bounded: C/POSIX plus the provisioned `de_DE`
  ISO-8859-1; other installed locales do not change matching (byte
  semantics), by the determinism contract.
- Traversal order is deterministic lexical (documented deviation from
  directory order); parse/usage errors exit 2 (documented deviation from
  GNU's 1).

## Recommendation for the shared ledger (not applied here)

Row `find` in `docs/posix-required-command-interfaces.tsv`: the
`go_evidence` column should append
`cmds/find/issue742_test.go#TestFindIssue7POSIXModeRequiresPathOperand;cmds/find/issue742_test.go#TestFindIssue7StatusAggregation;cmds/find/issue742_test.go#TestFindIssue7ExecSideEffectsAndBatching;cmds/find/issue742_test.go#TestFindIssue7LCAllPrecedenceForOKAffirmative;cmds/find/issue742_unix_test.go#TestFindIssue7OwnershipSeamUnassignedOwnerAndGroup;cmds/find/issue742_unix_test.go#TestFindIssue7NewerMissingReference;cmds/find/issue742_unix_test.go#TestFindIssue7UnreadableDirectoryTraversal`,
and `operand_rules` should be updated from "an expression-first
invocation is unspecified" to note the now-gated route: outside
POSIX mode the no-path `.` default remains a documented extension;
with POSIXLY_CORRECT (even empty) a path operand is required and
expression-first invocations are diagnosed. `evidence_state` may stay
`partial` (platform and locale-corpus boundaries above remain).

## Gates

- `gofmt -l cmds/find` clean; `go vet ./cmds/find` clean.
- `go test -count=20 ./cmds/find` — pass.
- `go test -race -count=5 ./cmds/find` — pass.
- Cross-builds of `./cmds/find`: linux/amd64, linux/arm64, darwin/arm64,
  darwin/amd64, windows/amd64, freebsd/amd64 — all pass;
  `./cmd/coreutils` also passes on the first five. The freebsd
  `cmd/coreutils` build fails in unrelated packages (`df`, `getconf`,
  `mknod`, `who`, `stat`) — verified identical on the base commit of
  this branch, pre-existing and outside this run's `cmds/find` scope.
