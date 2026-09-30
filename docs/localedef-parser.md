# Pure-Go localedef parser, store and collation

`cmds/localedef` is registered by the multicall. It compiles supported locale
sources into the Go-owned LOCPATH store; it no longer dispatches to an external
localedef provider. This is partial POSIX support, not full conformance.

The implementation is clean-room Go based on POSIX.1-2017 specifications and
black-box behavior, without copied or translated GNU/glibc/CUPS source.

```
localedef [-c] [-f charmap] [-i sourcefile] [-u codeset] name
```

Input defaults to invocation stdin; file and relative store paths honor
RunContext.Dir. Charmaps may be plain text or gzip; the default is ASCII.
Errors never create permanent output, including with -c. Warnings require -c
to create output (status 1); without it they return 4 without changing output.
Successful creation reports the compiled category names on stdout, one per line
in lexical order (including creation with warnings under `-c`).

`-u` supports case-insensitive `UTF-8` / `UTF8` and `ASCII` / `US-ASCII` /
`ANSI_X3.4-1968` / `ISO646-US`. UCS positions in the charmap encoding column use
`<Uhhhh>` or `<Uhhhhhhhh>` (four or eight hexadecimal digits); the same notation
without brackets is also accepted. For example, `<letter> <U00E9>` maps to the
two UTF-8 bytes C3 A9 under `-u UTF-8` and fails under `-u ASCII`. Positions in
ranges increment as UCS values before encoding, including across UTF-8 byte
length boundaries. Surrogates and positions above U+10FFFF fail with status 2.
Decimal, octal and hexadecimal byte escapes remain literal target bytes and
are validated against the chosen encoding. No source encoding is guessed.
The target supplies canonical codeset and width metadata (UTF-8: 1..4;
ASCII: 1..1), overriding conflicting charmap headers. Literal and copied
category data must also fit the explicit target. Unsupported targets and
unrepresentable data return 2 without creating or replacing an artifact.

A name containing `/` writes the JSON locale at exactly that pathname, without
adding `.json`; Windows also accepts its native separator. Relative paths use
RunContext.Dir, and private output needs neither HOME nor LOCPATH. Select the
pathname with LANG or LC_* to read it through `locale` and consuming utilities;
relative selection and category copy use the invocation directory too. Public
names retain the existing LOCPATH/default store and `.json` suffix. Private
files can be moved and selected by their new path. Absent compiled locales
retain existing host fallback; collation rejects malformed selected artifacts.
This closes the bounded interface work in story 162 (02d2bb56298e), not the
remaining locale grammar work. The behavioral source is the
[POSIX.1-2017 localedef specification](https://man7.org/linux/man-pages/man1/localedef.1p.html),
especially OPTIONS, OPERANDS, STDOUT and EXTENDED DESCRIPTION.

Compiled collation supports explicit character orders, collating elements and
symbols, forward/backward levels, IGNORE weights, literal and symbolic weights,
UNDEFINED expansion and category copy. Character identifiers accept literal
characters and hexadecimal, decimal or octal byte spellings as well as symbolic
names. Quoted one-to-many weights accept literal characters, symbolic names,
and mixtures. Literal segments resolve by encoded text, matching declared
collating elements longest first; symbolic references retain their named rank.
Sort, ls, comm, join and bracket consumers read compiled collation before host
fallback. The integrated bracket backend supports UTF-8 and multi-character
elements, range order and primary-weight equivalence. For a declaration named
<ch-digraph> with text ch, the bracket spelling is [[.ch.]].

Order ellipses expand the supplied charmap in encoded order, with checked
character endpoints. Empty weight operands use the element's own order.
Forward/backward levels accept `position`, preserving the number of ignored
elements encountered from the comparison direction. Explicit `UNDEFINED`
retains IGNORE, selected symbolic weights and ellipsis weights. Without an
explicit `UNDEFINED`, omitted coded characters produce a warning and are
appended in encoded order. The CLI requires `-c` to write a warned definition
(status 1); without `-c` it returns 4 and leaves output unchanged. Errors still
prevent output even with `-c`. Multi-level UNDEFINED defaults share the primary
weight and retain character order at subsequent levels.
Source: [POSIX.1-2017 XBD 7.3.2](https://pubs.opengroup.org/onlinepubs/9699919799.2018edition/basedefs/V1_chap07.html?view=full).

Encoding limits remain explicit: non-UTF-8 element encodings are unsupported.
Missing character or symbolic weight references fail. Legacy byte-table APIs
still reject multibyte/multi-character elements instead of truncating them;
the integrated full-element bracket path handles those elements. Inputs outside the compiled element set are errors; there
is no invented fallback order. Malformed compiled stores do not fall back to
the host. These limits remain visible even though ownership is now Go-only.

## Part3 recovery worker evidence

Interrupted run25 work was preserved in f1fed90a, then merged normally with
accepted integration base db0bb5eb in a64a0bd3. Both histories remain intact.
Fresh scoped terminal logs:

- /tmp/s340-recovery25-final-focused.log: 13 affected packages passed normal tests.
- /tmp/s340-recovery25-race.log: pkg/localedef, pkg/locale and pkg/collate passed race tests.
- /tmp/s340-recovery25-parser-final.log: localedef and collate passed after parser-audit syntax adjustment.
- /tmp/s340-recovery25-metadata-final2.log: all 58 interface metadata tests passed.

The applet matrix and interface generator checks passed with combined ownership
95/14/7 (availability), 87/22/7 (effective). The localedef interface ledger is
partial. Earlier /tmp/s340-red.log, focused/race logs and gate-final.log remain
historical evidence, including full-suite failures; no fresh full-suite or
whole-tree cross-platform claim is made. The parent independently gates this
candidate. Worker recovery did not merge canonical/main, push, publish or install.

`pkg/localedef` exposes `ParseCharmap`, `ParseSource`, `DefaultCharmap`, and
`Validate`. `Charmap.Symbols` maps names without angle brackets to encoded bytes.
`Source.Order` preserves category order; `Section.Entries` preserves entry order
and source lines. `Value.Kind` distinguishes words, numbers, symbols, strings,
bytes, separators, and ellipses. A string's `Parts` distinguishes literal bytes
from symbolic references; `Text` alone is not sufficient for compilation.
Character-map ranges are expanded; locale ellipses and conversion pairs remain
structured token sequences for the compiler.

The parser accepts the six POSIX categories, `copy`, custom CTYPE classes,
collating declarations, escape/comment directives, and continued lines. WIDTH
sections are skipped. Common additional system-source categories and time
keywords are retained without interpretation. Unknown categories and keywords
fail explicitly. Basic operand shapes and symbolic references are checked;
undefined references in CTYPE/COLLATE warn, while references in other categories
fail. Declared collating names must not duplicate charmap names.

Tests use original small examples. `TestSystemSource` reads the host's en_US and
UTF-8.gz files only when both exist. `TestProviderRejectsInvalidInput` uses the
provenance-checked external provider with a generated ASCII map and checks that
both implementations diagnose malformed input at line 2; it skips if that
provider is unavailable. Production code never executes another program.

## Story163 interrupted-run recovery evidence

The terminal run27 source/tests were preserved in `ac85da9f` with valid
Sprint340/story163 provenance. Recovery inspected that candidate and ran only
`./pkg/localedef ./pkg/locale ./pkg/collate ./cmds/localedef`, with
`GOFLAGS=-p=1`, `GOMAXPROCS=2`, and
`GOCACHE=/private/tmp/coreutils-issue27-go-cache`:

- `go test ... -count=1`: exit 0, `/tmp/issue27-recovery-normal.log`.
- `go test -race ... -count=1`: exit 0, `/tmp/issue27-recovery-race.log`.

These are worker checks, not acceptance or integrated gates. Prior red evidence
remains in `/tmp/issue27-parser-red.log` and `/tmp/issue27-forms-red.log`.
Interrupted broad attempts `/tmp/issue27-gate.log` and
`/tmp/issue27-gate-retry.log` remain unsuccessful/interrupted evidence and are
not claimed as passing. Recovery ran no broad suite, pre-push hook or install.

Tests cover forward/backward position with IGNORE and expanded weights,
encoded-order ellipses with boundary validation and aliases, leading/middle/
trailing empty weights, and explicit/implicit UNDEFINED defaults. The compiled
consumer fixture now uses `-c` and accepts warning status 1 only for omitted
coded characters: those partial-repertoire fixtures legitimately warn under
the required semantics. Its out-of-repertoire test uses a character absent
from the ASCII charmap; omitted ASCII characters now correctly acquire an
appended order. The private-path consumer fixture declares UNDEFINED to retain
its warning-free contract. Dedicated omission tests separately require exit 4
and no new output without -c, exit 1 and readable output with -c, and byte-for-
byte preservation of existing output after a forced compilation error.

Bracket/BRE/ctype/grep/sed implementation is untouched in this correction.
The later codeset scalar correction is not merged here; the parent combines
candidates and performs independent integration checks. No full-base conformance
claim is made; documented unsupported forms remain explicit.


## Story163 literal-form follow-up

The worker fast-forwarded to integrated `854b4678` before this correction,
retaining scalar-mapping and bracket work without editing bracket code.
Literal and numeric-character spellings now compile to the same order/weights
as symbolic spellings. Tests compare a/b/x and 1/2/3 against named equivalents,
including hexadecimal/decimal/octal bytes, scalar weights, quoted strings,
mixed literal/symbol strings, longest collating elements and missing references.
The red run is `/tmp/issue27-literal-red.log`. Final normal and race runs of
only pkg/localedef, pkg/collate and cmds/localedef completed with exit 0 in
`/tmp/issue27-literal-final-normal.log` and
`/tmp/issue27-literal-final-race.log`, with GOFLAGS=-p=1, GOMAXPROCS=2 and the
assigned issue27 cache. These are worker evidence; independent acceptance and
combined metadata generation remain the parent's responsibility.
