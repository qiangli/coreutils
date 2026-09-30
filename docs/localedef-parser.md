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
symbols, forward/backward levels, IGNORE weights, symbolic weight strings,
UNDEFINED expansion and category copy. Sort, ls, comm, join and byte bracket
consumers in grep/sed read compiled collation before host fallback.

Required LC_COLLATE grammar still has residuals: general order ellipsis,
position rules and empty weight operands are not fully implemented. Explicit
UNDEFINED has tested IGNORE/ellipsis-weight expansion, but absent UNDEFINED
does not yet warn and append omitted characters; comparison currently errors
on those characters. Parent review tracks these as required behavior gaps, not
optional extensions. Source: [POSIX.1-2017 locale definitions](https://pubs.opengroup.org/onlinepubs/9699919799.2018edition/basedefs/V1_chap07.html).

Additional unsupported forms fail explicitly:
non-UTF-8 element encodings, literal rather than ordered-symbol weight strings,
missing weight references, and byte brackets over multibyte or multi-character
collating elements. Inputs outside the compiled element set are errors; there
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
