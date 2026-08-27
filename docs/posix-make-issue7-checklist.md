# POSIX Issue 7 `make` conformance checklist

Normative reference: [POSIX.1-2017 `make`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/make.html).
GNU Make 4.3 and `theandrew168/make` (MIT) are development references only.

This checklist is the merge gate for replacing the managed external `make`.
Until every required row is implemented and exercised, `make` remains external-provider-owned and this package is not registered.

## Invocation and inputs

- [x] Options `-e`, `-f`, `-i`, `-k`, `-n`, `-p`, `-q`, `-r`, `-S`, `-s`, and `-t`, including grouped options and ordered `-k`/`-S` precedence.
- [x] Repeated `-f` files in command-line order and `-f -` from standard input.
- [x] Intermixed `macro=value` and target operands; command-line macro definitions win.
- [x] Default `./makefile`, then `./Makefile`; first ordinary target is the default goal.
- [x] `MAKEFLAGS` options and macro definitions are processed before command-line arguments and exported to recipes.
- [x] `SHELL` from the environment is ignored; the `SHELL` make macro defaults to the POSIX shell pathname.
- [x] Environment/makefile/command-line macro precedence, including `-e` and null environment values.
- [x] Command-line macros are exported to recipes without mutating the embedding process.
- [ ] XSI SCCS lookup and `PROJECTDIR` user-home resolution (not required by the base POSIX profile).

## Makefile language

- [x] Blank lines, comments, escaped comments, and non-recipe escaped-newline folding.
- [x] Recipe escaped-newline preservation and tab removal on the continuation line.
- [x] Nested `include` processing (at least 16 levels), expansion, and current-directory-relative paths.
- [x] Target rules, multiple targets, prerequisite accumulation, last command-bearing rule, and inline `; command`.
- [x] Macro definitions, lazy expansion, expansion in macro names, and `$(x)`/`${x}`/`$x`/`$$` forms.
- [x] POSIX suffix replacement expansions such as `$(OBJS:.c=.o)`.
- [ ] Recursive macro references fail clearly instead of silently expanding the recursive edge to empty (safety extension; POSIX leaves recursive-name behavior unspecified).
- [x] Inference-rule redefinition and empty inference rules.

## Rules and update algorithm

- [x] Ordered recursive prerequisite updates and cycle detection.
- [x] Timestamp comparison, including equal timestamps and targets successfully updated without creating a file.
- [x] Explicit rules with prerequisites but no recipes.
- [x] `.DEFAULT`, `.IGNORE`, `.POSIX`, `.PRECIOUS`, `.SILENT`, and `.SUFFIXES` semantics.
- [x] Double- and single-suffix inference in suffix-list order.
- [x] Required default macros, suffix list, and C/FORTRAN/shell inference rules; `-r` clears them.
- [x] `$@`, `$%`, `$?`, `$<`, `$*` and their `D`/`F` variants.
- [x] Archive-member target parsing and member timestamp comparison.
- [ ] XSI `.SCCS_GET` retrieval and SCCS `~` inference rules (not required by the base POSIX profile).

## Recipe execution and results

- [x] One shell per logical recipe line, with the make invocation's virtual cwd/environment.
- [x] Shell `-e` for non-ignored recipes; no `-e` for ignored recipes.
- [x] `-`, `@`, and `+` prefixes in any combination.
- [x] `-n`, `-q`, and `-t` still execute `+` recipes.
- [x] `-i`/`.IGNORE`, `-s`/`.SILENT`, `-k`/`-S`, command echoing, and diagnostics.
- [x] `-q` exit 0/1/>1 distinction and no updates except `+` recipes.
- [x] `-t` touches eligible targets, prints touch messages unless silent, and does not touch recipe-less targets.
- [x] A no-work invocation reports that no action was taken.
- [x] Context cancellation removes a partially updated non-directory, non-precious target unless `-n`, `-p`, or `-q` applies.
- [x] Standalone-process SIGHUP/SIGINT/SIGQUIT/SIGTERM cleanup and boundary re-raise contract; embedded invocations use context cancellation because process-global signal handlers are unsafe.

## Evidence still required before ownership migration

- [ ] Focused tests cover every checked behavior above on all supported build targets.
- [x] Nine-case POSIX-compatible differential corpus passes against a locally built, official GNU Make 4.3 oracle.
- [ ] Profile D VSC/PCTS `make` tests pass on a pinned disposable runner.
- [x] Full short repository tests, cross-vet, applet matrix, and POSIX manifest pass while ownership remains external.
- [ ] Only after those gates: register the package, remove the external-provider manifest/build row, regenerate ownership documents, and update the umbrella pin.
