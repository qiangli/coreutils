# Profile C POSIX interface audit: file, od, who

Scope: the Profile C (stock GNU Bash 5.3 `sh` + staged Bashy Go userland)
Go-selected commands `cmds/file`, `cmds/od`, and `cmds/who`, audited against
POSIX.1-2008 Issue 7 (2016 Edition) at base `9e9dc19`. The issue supplies
retained diagnostic-blocker counts (`file` 14, `od` 11, `who` 11) from a
pre-Sprint-79 evidence state. This audit re-verifies the current Issue 7
interface from source, focused tests, and selected current-source binary
probes, and attributes the residual classes that remain. GNU 9.11 extension
parity is out of scope.

## Treatment of the retained diagnostic-blocker counts

The numerical counts and their diagnostic/message-catalog classification come
from the Issue 774 assignment. No durable, non-licensed in-tree ledger records
the individual entries, producing run, checkpoint, or source revision, so this
audit cannot independently reconstruct or validate those historical numbers.
It does not supersede them as evidence. Sprint 79 did withdraw the universal
translated-diagnostic blocker as too broad
([sprint-79-consolidated.md](sprint-79-consolidated.md), "Fail-closed evidence
decision"), on four grounds that apply to the issue-supplied classification:

1. `ENVIRONMENT VARIABLES` clauses make `LC_ALL`→category precedence and
   honoring a recognized locale mandatory; none of `file`, `od`, or `who` has
   normative behavior that *depends on* `LC_MESSAGES` data (no
   affirmative-response parsing, no locale-selected message alternatives).
2. XCU's `STDERR` default uses *should*, not *shall*, for unspecified-format
   diagnostics; the deterministic POSIX-locale English of the agent contract
   satisfies every observable diagnostic requirement each command actually
   has (verified live below).
3. `NLSPATH` catalog lookup is XSI-`catopen()`-conditional and recorded as
   `xsi:NLSPATH` in the ledger, not a Base-profile obligation of these Go
   rows.
4. No clause requires a utility to ship translated catalogs; their absence is
   a localization product gap, not a failed interface.

Therefore the issue-supplied counts are not evidence of a current Bashy-owned
interface defect. Because their entries are not reconstructable, this audit
does not claim entry-by-entry closure. Current source and tests instead leave
the per-command rows fail-closed with the locale-provider, platform,
login-database, and integration boundaries attributed below; no current
product defect was reproduced by the selected checks.

## file — `file [-dh] [-M file] [-m file] file...` ; `file -i [-h] file...`

Authoritative closure: [file-issue746.md](file-issue746.md) (Issue 746,
manager-reviewed). Re-verified at this base:

- All five required options register with exact semantics: `-h`
  (`file.go:86`), `-i` (`file.go:87`), `-d` (`file.go:89`), `-M`
  (`file.go:90`), and `-m` (`file.go:91`). The `-d`/`-M`/`-m` options feed the
  ordered, position-sensitive source plan; `-i` is independent and rejects
  combination with any of those source options (`file.go:99-100`). Usage
  (`file.go:33-35`) states both synopsis forms.
- Selected current-source multicall probes: missing operand →
  `file: missing file operand` + status 2; `-i` combined with `-m` →
  `file: -i cannot be combined with -d, -M, or -m` + status 2; nonexistent
  operand → stdout line `cannot open "<name>" (No such file or directory)`
  with aggregate status 0, exactly the Issue 7 "does not by itself affect the
  exit status" rule; `-h` on a dangling symlink → `symbolic link to
  <target>`, status 0.
- Focused suite `cmds/file` passes; `go vet` clean.

Residuals (precisely attributed): `LC_CTYPE` provider is the bounded carried
corpus via `RunContext`, not `nl_langinfo` (locale-provider boundary, shared
with `cut`/`expand`/`paste`/`iconv`); device major/minor wording is
platform-specific (`file_unix.go` vs `file_other.go`) with non-Unix targets
using generic wording (platform boundary); type-string breadth beyond the
required categories is implementation-defined. None is reproduced as a defect.

## od — base `od [-v] [-A address_base] [-j skip] [-N count] [-t type_string]... [file...]`, XSI `od [-bcdosx] [file] [[+]offset[.][b]]`

- Base options `-A` (`od.go:158`), `-t` (`od.go:159`, repeatable
  `StringArrayP`), `-N` (`od.go:160`), `-j` (`od.go:161`), `-v` (`od.go:171`)
  and XSI aliases `-b`/`-c`/`-d`/`-o`/`-s`/`-x` (`od.go:166-184`) all
  register with documented semantics; `expandFormatArgs` keeps the XSI
  single-letter route distinct from literal `-t` via `aliasMark`.
- XSI offset-operand gating (`od.go:194-227`) implements the Issue 7 OPERANDS
  rule exactly: the final operand is an offset only when there are at most two
  operands, none of `-A`/`-j`/`-N`/`-t`/`-v` was specified, and the operand
  is `+`-prefixed or the digit-prefixed second of exactly two operands.
- Live binary probes: `od f 10` skips `0o10` bytes; `od f +10.` skips decimal
  10 with the start offset still rendered `0000012` in the default octal
  radix; `od f 10b` applies the ×512 multiplier and produces no output past
  EOF with status 0; `od -A x f 10` closes the gate and treats `10` as a file
  operand (diagnostic, continue); one missing file among operands → aggregate
  status 1; invalid `-A z`, `-j x`, and `-t q` each fail loudly naming the
  value with status 2.
- Existing public tests pin every probed behavior:
  `TestODTraditionalOffsetRadix`, `TestODSkipPastEOF`, `TestODXSIOffsetGating`,
  `TestODXSITwoOperandNumericOffset`, `TestODMissingFileContinues`,
  `TestODRejectsBadFormat`. Focused suite `cmds/od` passes; `go vet` clean.

Residuals (precisely attributed): `LC_CTYPE`/`LC_NUMERIC` providers are the
bounded carried corpus (UTF-8 aliases, Latin-1) and Unicode printability
tables are bounded (locale-provider boundary); translated catalogs are absent
(localization product gap, superseded blocker class). None is reproduced as a
defect.

## who — base `who [-mTu] [file]`, XSI selectors, `who am i` / `who am I`

- Base options `-m` (`who.go:36`), `-T` (`who.go:27`), `-u` (`who.go:26`) and
  XSI `-a -b -d -H -l -p -q -r -s -t` (`who.go:22-35`) all register;
  `parseOperands` (`who.go:240-256`) enforces the exact POSIX operand grammar
  — zero or one `file`, or the two-operand `am i`/`am I` form — and returns a
  usage error for anything else rather than guessing.
- Live binary probes: `who foo bar` → `who: extra operand "bar"` + status 2;
  `who -q -H` prints the quick list with `# users=` and no heading (selector
  precedence); `who am i` and `who -m` exit 0 and are silent when stdin is
  not a terminal (no invoking-terminal line exists to report).
- Existing public tests: `TestWhoOperands`, `TestWhoQuietIgnoresOtherOptions`,
  `TestWhoTExactNoOptionalComment`, `TestWhoAllIsExactAndTruthful`,
  `TestWhoLCtimeProviderAndFailClosedResidual`, plus the binary-ABI and
  native-Linux differential suites. Focused suite `cmds/who` passes;
  `go vet` clean.

Residuals (precisely attributed): the login database is
implementation-defined; the `file` operand substitutes a utmp/utmpx-format
database covered by `testdata` fixtures rather than a live host database
(login-database integration boundary); `-m`/`am i` depend on a controlling
terminal for stdin (terminal integration boundary); `LC_TIME` provider is the
carried corpus. Tty message-state lookup uses `os.Stat` and the group-write
permission bit on non-Windows targets (`who.go`, `who_writable_other.go`),
while Windows reports the state as unknown (`who_writable_windows.go`). Stdin
TTY discovery and access-time lookup have Linux, Darwin, and Windows providers;
other targets return unavailable, yielding no `-m` match and an unknown idle
status (`who_tty_*.go`). These are platform/integration boundaries, not a
reproduced product defect.

## Changes made

- None to product source. Source registration and the cited focused public
  tests cover the audited interface, and selected behaviors were probed using
  a direct current-source multicall build. No Bashy-owned gap was reproduced,
  so per the audit policy no speculative test or code change was added.
- Probe provenance: from repository revision `9e9dc19`, build with
  `go build -o /tmp/coreutils ./cmd/coreutils`, then invoke the `file`, `od`,
  and `who` operands described above through that binary. This is a
  current-source check of the Go userland selected by Profile C, not evidence
  of a staged end-to-end Profile C integration run.
- The issue-supplied `file` 14 / `od` 11 / `who` 11 counts cannot be
  independently reconstructed from a durable non-licensed in-tree artifact;
  the command rows therefore remain fail-closed on the current residuals.

## Matrix reconciliation owed (manager)

No source, test, or file-count change was made in this audit, so no
`docs/posix-required-command-interfaces.tsv`, `docs/applet-matrix.*`, or
other shared/generated manifest regeneration is owed by this work; the
manager regenerates matrices independently.
