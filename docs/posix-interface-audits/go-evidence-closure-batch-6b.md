# POSIX Go evidence closure: wave 6B

Wave 6B covers the Go-owned `locale`, `ls`, and `mesg` interfaces against
POSIX.1-2016 Issue 7. All three rows move from `unverified` to `partial`.
The independent review deliberately separates normative requirements from
Bashy's deterministic or upstream-compatible choices.

## Verdicts and residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `locale` | [Issue 7 locale](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html) | partial | Environment variable query formatting, category/keyword queries, `charmap`, `-a`, `-m`, `-c`, `-k`, `--` option termination, and stdout write error status (status 1) are covered. Built-in compiled locale data is limited to C/POSIX and `de_DE.ISO-8859-1` LC_TIME / LC_MESSAGES; uncarried locales fail closed by design rather than returning dummy C values. Full host locale catalog database access and translated diagnostics remain absent. |
| `ls` | [Issue 7 ls](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html) | partial | Full POSIX option set (`-A`, `-C`, `-F`, `-H`, `-L`, `-R`, `-S`, `-a`, `-c`, `-d`, `-f`, `-g`, `-i`, `-k`, `-l`, `-m`, `-n`, `-o`, `-p`, `-q`, `-r`, `-s`, `-t`, `-u`, `-x`, `-1`), `--` option termination, and `-s` block-size defaulting (512-byte blocks in POSIX mode `POSIXLY_CORRECT`, 1024-byte blocks outside POSIX mode or with `-k`) are covered. Multi-column formatting assumes non-tty line width from `-w` or `COLUMNS` or 80-column fallback; terminal-dependent color/hyperlink and `--classify=auto` fail closed; `LC_COLLATE` ordering for non-C locales and translated diagnostics remain absent. |
| `mesg` | [Issue 7 mesg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html) | partial | Terminal permission query (stdout `is y`/`is n`, exit 0 for allowed / 1 for denied), setting `y` (g+w) or `n` (g-w), `--` option termination, non-terminal/error handling (exit status 2), and stdout write error handling are covered. Terminal message permissions are non-functional on Windows (refuses with exit status 2 by design). Translated diagnostics and `NLSPATH` catalog lookup remain absent. |

## Evidence added

- `locale`: `cmds/locale/locale_test.go#TestLocaleDoubleDashTerminatesOptions` and `#TestLocaleOutputWriteErrorsFail` pin `--` option termination and stdout write failure exit status 1.
- `ls`: `cmds/ls/ls_posix_test.go#TestSizeBlocksPOSIX512ByteDefault` and `#TestLsDoubleDashTerminatesOptions` pin 512-byte block sizing for `ls -s` under `POSIXLY_CORRECT=1` and `--` option termination.
- `mesg`: `cmds/mesg/mesg_test.go#TestMesgDoubleDashTerminatesOptions` and `#TestMesgOutputWriteError` pin `--` option termination and stdout write error exit status 2.

Focused count-10 and race tests pass for all three packages. Windows vet and cross-platform checks pass.
