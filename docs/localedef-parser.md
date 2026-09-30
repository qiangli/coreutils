# Pure-Go localedef parser (story 155)

`cmds/localedef` is an embeddable parse/validate command. Importing it registers
`localedef` with `tool`; it is not yet imported by the multicall. The existing
external-provider routing and the applet inventory remain unchanged.

The implementation is clean-room Go based on POSIX.1-2017
[XCU localedef](https://pubs.opengroup.org/onlinepubs/9699919799.2018edition/utilities/localedef.html)
and [XBD locale definitions](https://pubs.opengroup.org/onlinepubs/9699919799.2018edition/basedefs/V1_chap07.html).
No upstream implementation or locale data is included.

```
localedef [-c] [-f charmap] [-i sourcefile] [-u codeset] name
```

Source input defaults to invocation stdin. Files resolve against `RunContext.Dir`.
An explicit charmap can be plain text or gzip; without `-f`, the portable
character names use ASCII. Successful validation is silent. Status is 0 for
valid input, 1 for warnings, and 4 for errors; diagnostics go to stderr and
include the input filename and source line when available.

This is the first of three delivery stories, not a locale compiler. No output
files are created. `-c`, `-u`, and the locale name are parsed into `Options` for
the next compilation stage; `-u` does not yet transcode encodings. The warning
status is 1 with or without `-c` during this parse-only stage. Copy targets are
retained without being opened or resolved. Category completeness, character
class semantics, locale-store output, and compiled collation belong to the
following stories.

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
