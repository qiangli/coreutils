# POSIX Go evidence closure: wave 6B

Wave 6B covers the Go-owned `locale`, `ls`, and `mesg` interfaces against
POSIX.1-2016 Issue 7. All three rows move from `unverified` to `partial`.
The independent review deliberately separates normative requirements from
Bashy's deterministic or upstream-compatible choices.

## Verdicts and residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `locale` | [Issue 7 locale](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html) | partial | Environment variable query formatting, category/keyword queries, `charmap`, `-a`, `-m`, `-c`, `-k`, `--` option termination, and stdout write error status (status 1) are covered. `-a` now advertises only the fully queryable public C and POSIX locales instead of unrelated host locale-directory entries. Partial `de_DE.ISO-8859-1` LC_TIME / LC_MESSAGES fixtures are not advertised as a complete public locale. Full host locale catalog database access and translated diagnostics remain absent. |
| `ls` | [Issue 7 ls](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html) | partial | The full POSIX option set, `--`, POSIX block sizing, non-C `LC_COLLATE`/`LC_CTYPE`, and Issue 777's last-`-H`/`-L` ordering across clusters/spellings are covered. A sticky checked writer propagates immediate errors, nil-error short writes, and late failures across every listing format, help/version, continuation, and recursion. Remaining: non-C `LC_TIME`, terminal width/capability discovery, and non-Unix runtime metadata behavior. |
| `mesg` | [Issue 7 mesg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html) | partial | Terminal permission query (stdout `is y`/`is n`, exit 0 for allowed / 1 for denied), setting `y` (g+w) or `n` (g-w), `--` option termination, RunContext stdin/stdout/stderr terminal selection, rejection of non-terminal character devices, non-terminal/error handling (exit status 2), and stdout write error handling are covered. Terminal message permissions are non-functional on Windows (refuses with exit status 2 by design). Translated diagnostics and `NLSPATH` catalog lookup remain absent. |

## Evidence added

- `locale`: `cmds/locale/locale_test.go#TestLocaleDoubleDashTerminatesOptions` and `#TestLocaleOutputWriteErrorsFail` pin `--` option termination and stdout write failure exit status 1.
- `ls`: `cmds/ls/ls_posix_test.go#TestSizeBlocksPOSIX512ByteDefault`, `cmds/ls/ls_locale_test.go#TestLsLocaleCollationPrecedence`, `#TestLsLocaleCTypePrecedenceForQuestionMark`, and `#TestLsDoubleDashTerminatesOptions` pin block sizing, locale category precedence, and `--` option termination.
- `mesg`: `cmds/mesg/mesg_test.go#TestMesgDoubleDashTerminatesOptions` and `#TestMesgOutputWriteError` pin `--` option termination and stdout write error exit status 2.

Focused count-10 and race tests pass for all three packages. Focused Windows,
Linux, and Darwin vet checks pass; the aggregate cross-platform gate is run
after root regenerates the shared artifacts.
