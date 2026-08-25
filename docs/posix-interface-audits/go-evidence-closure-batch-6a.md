# POSIX interface evidence closure - Go batch 6a

Scope is exactly the Go-owned `awk`, `getconf`, and `grep` applets.
Normative source is POSIX.1-2016 Issue 7, per `docs/reference-policy.md`.
GNU, Gawk, and GNU Grep behavior is extension-only and was not used to promote
the POSIX ledger state.

The three rows in `docs/posix-required-command-interfaces.tsv` move from
`unverified` to `partial`. None is promoted to `verified`.

## awk

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html

Evidence added in this batch:

- `cmds/awk/awk_test.go#TestAwkPOSIXInterfaceProgramFileAndAssignments`
  covers repeatable `-f`, `-v assignment`, input-file operands, and
  `name=value` operand timing before the following input.
- `cmds/awk/awk_test.go#TestAwkPOSIXProgramFromStdinAndEmptyProgram` covers
  `-f -` program-source stdin and the valid empty program reading no input.
- `cmds/awk/awk_test.go#TestAwkPOSIXInvalidAssignmentAndMissingInput` covers
  invalid `-v assignment` diagnostics and inaccessible input status.

Existing evidence retained in the row covers the basic synopsis, `-F`, program
files, POSIX numeric formatting cases, ERE leftmost-longest/dot-newline
semantics, regex endpoint routing, LC_CTYPE/LC_COLLATE regex tables, and locale
lifecycle errors.

Residuals:

- `LC_NUMERIC` is passed through `ENVIRON`, but decimal input, string-number
  conversion, and formatted output are not wired to a locale numeric provider.
- Full POSIX awk language conformance is inherited from GoAWK plus focused
  local fixes; the repository does not yet carry exhaustive Issue 7 language
  evidence for every expression, built-in, redirection, and error case.
- Message catalog behavior for `LC_MESSAGES`/`NLSPATH` is not implemented;
  diagnostics are deterministic C-locale text.

Disposition: `partial`.

## getconf

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html

Evidence added in this batch:

- `cmds/getconf/getconf_test.go#TestPOSIXArityAndOptionForms` covers empty
  `-v`, missing `-v` option-argument diagnostics, `system_var` versus
  `path_var pathname` arity, and wrong-kind variable diagnostics.

Existing evidence retained in the row covers host differentials on Darwin,
known-versus-unknown variable behavior, POSIX compile-time minimums, inventory
classification, Darwin confstr/pathconf/sysconf adapters, Windows fail-closed
behavior, Linux non-invented libc values, `-a` as an extension, unsupported
specifications, and missing operands.

Residuals:

- Linux has no libc-backed sysconf/confstr adapter in this pure-Go applet, so
  many known non-minimum names intentionally report `undefined` rather than a
  host libc value.
- Windows has no POSIX sysconf/pathconf/confstr ABI, so it fails closed for
  capability claims except standard compile-time minimum names.
- `-v specification` is accepted only for platform adapters that can honestly
  identify the requested programming environment; there is no cross-platform
  synthetic programming-environment table.
- Message catalog behavior for `LC_MESSAGES`/`NLSPATH` is not implemented;
  diagnostics are deterministic C-locale text.

Disposition: `partial`.

## grep

Reference: https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/grep.html

Evidence added in this batch:

- `cmds/grep/grep_test.go#TestGrepPOSIXPatternListAndOptionPrecedence` covers
  an empty `-e pattern_list`, `-q` suppression of normal output, and missing
  `-f` option-argument diagnostics.

Existing evidence retained in the row covers stdin and `-` operands, required
options `-E`, `-F`, `-c`, `-e`, `-f`, `-i`, `-l`, `-n`, `-q`, `-s`, `-v`, and
`-x`, newline-separated pattern lists, pattern files, POSIXLY_CORRECT operand
parsing, file diagnostics, exit status 0/1/>1, BRE/ERE conformance fixes,
RE_DUP_MAX-scale intervals, leftmost-longest match extents, and C/POSIX plus
built-in de_DE ISO-8859-1 locale precedence.

Residuals:

- Locale-sensitive regex behavior is implemented only for C/POSIX and the
  built-in de_DE ISO-8859-1 certification locale path; arbitrary public locale
  archives are not loaded for `grep`.
- `LC_MESSAGES`/`NLSPATH` catalog behavior is not implemented; diagnostics are
  deterministic C-locale text.
- GNU extensions such as recursive search, binary-file summaries, context, and
  `-o` are preserved outside the POSIX evidence claim and do not promote the
  Issue 7 row to `verified`.

Disposition: `partial`.

## Gate

Focused package gate after adding evidence:

```sh
go test ./cmds/awk ./cmds/getconf ./cmds/grep
```

The full requested gate for this worker is recorded in the final response and
commit.
