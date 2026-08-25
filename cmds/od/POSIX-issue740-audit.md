# `od` POSIX locale closure (Sprint 79 issue 740)

Normative reference: [POSIX.1 Issue 7 (2016), XCU `od`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html).
GNU compatibility is outside this issue.

## Closed requirements

| Issue 7 requirement | Implementation and focused evidence |
| --- | --- |
| `LC_ALL` > category > `LANG` > implementation default | `runWithProfile` resolves only invocation-owned `RunContext.Env` through `pkg/locale.Resolve`; `TestODCTypeLocaleRenderingAndPrecedence` and `TestODNumericLocalePrecedenceAndScientificRadix` cover precedence and empty `LC_ALL`. No process locale is read or changed. |
| `LC_CTYPE` determines `c` byte-to-character interpretation | The bounded model implements C/POSIX single-byte behavior, C/POSIX and `de_DE` UTF-8 aliases, and `de_DE.ISO-8859-1`. `TestODCTypeLocaleRenderingAndPrecedence` distinguishes all three encodings and proves ISO-8859-1 bytes are not transcoded. |
| Printable multibyte characters occupy the first byte field and `**` occupies each continuation field | `renderCFields` preserves one field per source byte, including a character crossing output-group boundaries. `TestODCTypeUTF8ContinuationAcrossOutputGroups` pins the exact product. |
| Required escapes and per-byte octal rendering for other non-printable characters | `cCharSingleByte` handles NUL plus `\\a`, `\\b`, `\\f`, `\\n`, `\\r`, `\\t`, and `\\v`; valid non-printable multibyte characters and malformed UTF-8 remain exact per-byte octal fields. `TestODCTypeUTF8ContinuationAcrossOutputGroups` covers the discriminating products. |
| Streaming input is not delayed by speculative multibyte lookahead | `peekUTF8Continuation` asks only for bytes missing from a demonstrably incomplete suffix and stops as soon as the sequence is decidable. `TestODUTF8FullASCIIGroupDoesNotWaitForLookahead` holds an `io.Pipe` open after a complete ASCII group and requires output before EOF; `TestODUTF8BoundaryLookaheadIsExact` covers complete, incomplete, stray, and invalid suffixes. |
| `LC_NUMERIC` selects the floating-output radix | Every `f` path localizes only the mantissa radix: IEEE32, IEEE64, x87 80-bit in 12/16-byte ABIs, IEEE binary128, and IBM double-double. `TestODNumericLocaleAllFloatingTypesAndABIs` covers bare `f`, `fF`/`f4`, `fD`/`f8`, `fL`, explicit `f12`, and explicit `f16`; `TestODNumericLocalePrecedenceAndScientificRadix` covers exponent output and POSIX/de_DE radix selection. |
| Unsupported locale data cannot silently inherit C behavior | A relevant unsupported `LC_CTYPE` or `LC_NUMERIC` fails with status 1 before any operand is opened. An unsupported category that the selected formats do not use is not consulted. `TestODLocaleCategoriesFailClosedOnlyWhenUsed` pins both halves. |

## Deliberate bounded-provider residuals

The implementation does not claim an installed host locale database. Its
carried `LC_CTYPE` inventory is C/POSIX, their UTF-8 aliases,
`de_DE.UTF-8`, and `de_DE.ISO-8859-1`; its carried `LC_NUMERIC` inventory is
C/POSIX and the reviewed `de_DE` aliases. Other effective locale names fail
closed. UTF-8 printability follows Go's pinned Unicode tables, not a host
`iswprint_l()` database. Translated `LC_MESSAGES` diagnostics and `NLSPATH`
catalog loading remain absent under the repository's consolidated diagnostic
policy. These boundaries must remain explicit in the shared Sprint 79 ledger;
they are not grounds to claim arbitrary installed-locale coverage.

## Gates run in the issue workspace

- `go test ./cmds/od -count=20`
- `go test -race ./cmds/od -count=5`
- `go vet ./cmds/od`
- `go build ./cmds/od`

The repository-wide cross-platform, applet-matrix, manifest, and aggregate
gates are run after review against current `main`, because this workspace was
cut from `64ce4c5` while shared evidence advanced independently.
