# `ls` / `stty` POSIX.1-2016 Issue 7 Audit

Scope: the directory listing utility `ls` and terminal state utility `stty` against The Open Group POSIX.1-2008 Issue 7, 2016 Edition utility pages, audited end-to-end against `cmds/ls` and `cmds/stty` as of main `9e9dc19`.

* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html>
* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html>

Both rows remain **partial** under the consolidated Sprint 79 fail-closed policy while shared process-boundary, terminal capabilities, and non-Unix platform evidence remain incomplete. The stale metrics (from earlier diagnostic reports stating: `ls 115 TPs/20 blockers; stty 101/17 blockers/14 manual`) are superseded by the current, far newer implementation on main, where required C/POSIX-locale features and exact command option/operand/status constraints are covered. The absence of shipped translated catalogs is recorded as a localization product gap, not treated as a command-interface blocker by itself.

## 1. `ls` — Options, Operands, and the `--` Special Token

POSIX `ls` defines the full option set (`-A`, `-C`, `-F`, `-H`, `-L`, `-R`, `-S`, `-a`, `-c`, `-d`, `-f`, `-g`, `-i`, `-k`, `-l`, `-m`, `-n`, `-o`, `-p`, `-q`, `-r`, `-s`, `-t`, `-u`, `-x`, `-1`). The option parser and format selections operate locally per invocation.

The `--` option termination token is correctly parsed to end option decoding, ensuring any subsequent arguments beginning with `-` are treated as operands.

Operands default to `.` (the current directory) when empty. Access failures on individual operands produce diagnostics on standard error and set a non-zero exit status, while still allowing the utility to continue processing remaining operands.

Evidence: `TestDefaultSortAndOnePerLine`, `TestMixedOperandsHeaders`, `TestNonexistentOperand`, `TestLsDoubleDashTerminatesOptions`.

## 2. `ls` — Command-Line Symbolic Link Dereferencing

POSIX `-H` and `-L` control symbolic link dereferencing:
- `-L` evaluates all symbolic links named as operands or encountered during traversal.
- `-H` evaluates symbolic links specified on the command line.

A confirmed Bashy-owned gap has been fixed: command-line symbolic links pointing to directories were not being dereferenced under `-H` / `-L` when `-d` (directory only) was specified, incorrectly displaying the symlink metadata instead of the referent directory information. This is now resolved: the referent directory metadata (such as permissions, size, and type `d`) is correctly retrieved and displayed.

Additionally, for dangling symbolic links (where dereferencing is attempted but the target does not exist), the utility gracefully falls back to displaying the symlink's own metadata along with its target arrow (`-> target`) in long listing format, matching POSIX and GNU behaviors.

Evidence: `TestLsDereferenceCommandLineSymlinks`, `TestDereferenceDirectoryEntries`.

## 3. `ls` — Sizing, Radix, and Columns

Block sizes for `-s` default to 512-byte blocks when `POSIXLY_CORRECT` is set (as required by Issue 7), but use 1024-byte blocks by default or when `-k` is specified. Radix formatting for file sizes adapts to `-h` and `--si` formats in powers of 1024 and 1000, respectively. Column width formatting queries `-w` or the `COLUMNS` environment variable, falling back to 80 columns when running off-tty.

Evidence: `TestSizeBlocksPOSIX512ByteDefault`, `TestBlockSize`, `TestColumnsHonorsColumnsEnv`.

## 4. `stty` — Options and Mutually Exclusive Styles

POSIX `stty` defines exactly two options: `-a` (write all current settings) and `-g` (write settings in an stty-reusable hex format). Standard input must be associated with a terminal. In line with the standard, `-a` and `-g` are mutually exclusive, and settings cannot be passed when either output style is selected.

Evidence: `TestSttyRejectsNonTTY`, `TestSttyRejectsConflictingOutputStylesBeforeTTY`, `TestSttyRejectsSettingsWithOutputStyleBeforeTTY`.

## 5. `stty` — Settings and Control Character Operands

All POSIX required terminal modes (control, input, output, local, and combination modes), baud rates/speeds, and control character settings (including `min`, `time`, and disabled control characters using `undef`) are supported and applied atomically. The validation phase checks the complete operand sequence before performing any state modification, preventing partial terminal configuration changes on error.

Evidence: `TestSttyRowsColsRejectsOverflow`, `TestSttyRowsAppliesWindowSize`, `TestSttyColumnsAppliesWindowSize`, `TestSttyRequiredReportsPropagateWriteErrors`, `TestApplyTermiosModeRaw`, `TestApplyTermiosValueMinTime`.

## 6. Exit Status

All exits conform to POSIX partitions:
- `ls` exits `0` on success and `>0` (specifically `1` or `2`) on directory open, stat, or operand errors.
- `stty` exits `0` on successful query/modification and `>0` (specifically `1`) on non-tty stdin, conflicting options, or execution failures.
Standard output write errors trigger diagnostics and exit status `1` in both commands.

Evidence: `TestNonexistentOperand`, `TestUnknownFlag`, `TestSttyRejectsNonTTY`, `TestSttyRequiredReportsPropagateWriteErrors`.

## 7. Platform Disposition and Residuals

Both implementations are pure Go. The residual issues that keep these rows `partial`:
- **Locale Constraints:** `LC_COLLATE` sorting for non-C locales is absent. Non-C `LC_TIME`/`LC_MESSAGES` rendering is unsupported. Missing localized message catalogs are treated as a product localization gap.
- **Terminal Capabilities:** Real terminal output formatting (e.g. multi-column wrapping) relies on PTY/tty size or `COLUMNS` overrides.
- **Platform Limitations:** `stty` has no termios/PTY support on Windows by design (fails gracefully with exit status 1). `ls` inode, link counts, and block counts fall back to 0 on Windows.
- **Dangling symlink metadata:** Unresolvable symlinks inside a directory under `ls -lL` log a diagnostic and return exit status 1, falling back to the symlink's own metadata.

## 8. Gate Record

Local gate verification runs for this issue:

```sh
go test -count=20 ./cmds/ls ./cmds/stty
POSIXLY_CORRECT=1 go test -count=20 ./cmds/ls ./cmds/stty
go test -race -count=5 ./cmds/ls ./cmds/stty
POSIXLY_CORRECT=1 go test -race -count=5 ./cmds/ls ./cmds/stty
go vet ./cmds/ls ./cmds/stty
GOOS=linux GOARCH=386 go vet ./cmds/ls ./cmds/stty
GOOS=linux GOARCH=amd64 go vet ./cmds/ls ./cmds/stty
GOOS=darwin GOARCH=arm64 go vet ./cmds/ls ./cmds/stty
GOOS=windows GOARCH=amd64 go vet ./cmds/ls ./cmds/stty
GOOS=aix GOARCH=ppc64 go vet ./cmds/ls ./cmds/stty
```
