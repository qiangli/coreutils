# `ls` / `stty` POSIX.1-2016 Issue 7 Audit

Scope: the directory listing utility `ls` and terminal state utility `stty` against The Open Group POSIX.1-2008 Issue 7, 2016 Edition utility pages, audited end-to-end against `cmds/ls` and `cmds/stty` as of main `9e9dc19`.

* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html>
* <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html>

Both rows remain **partial** under the consolidated Sprint 79 fail-closed policy while shared process-boundary, terminal capabilities, and non-Unix platform evidence remain incomplete. Earlier diagnostic metrics (`ls 115 TPs/20 blockers; stty 101/17 blockers/14 manual`) describe an older implementation and are not current counts. This audit records the verified areas and residuals below; it does not claim source-complete conformance. The absence of shipped translated catalogs is recorded as a localization product gap, not treated as a command-interface blocker by itself.

## 1. `ls` — Options, Operands, and the `--` Special Token

POSIX `ls` defines the full option set (`-A`, `-C`, `-F`, `-H`, `-L`, `-R`, `-S`, `-a`, `-c`, `-d`, `-f`, `-g`, `-i`, `-k`, `-l`, `-m`, `-n`, `-o`, `-p`, `-q`, `-r`, `-s`, `-t`, `-u`, `-x`, `-1`). The option parser and format selections operate locally per invocation.

The `--` option termination token is correctly parsed to end option decoding, ensuring any subsequent arguments beginning with `-` are treated as operands.

Operands default to `.` (the current directory) when empty. Access failures on individual operands produce diagnostics on standard error and set a non-zero exit status, while still allowing the utility to continue processing remaining operands.

Evidence: `TestDefaultSortAndOnePerLine`, `TestMixedOperandsHeaders`, `TestNonexistentOperand`, `TestLsDoubleDashTerminatesOptions`.

## 2. `ls` — Command-Line Symbolic Link Dereferencing

POSIX `-H` and `-L` control symbolic link dereferencing:
- `-L` evaluates all symbolic links named as operands or encountered during traversal.
- `-H` evaluates symbolic links specified on the command line.

They form a mutually exclusive option set: the last `-H` or `-L` specified
determines behavior. The implementation resolves one ordered mode across
short-option clusters and long spellings. Both modes dereference command-line
operands, while only a final `-L` dereferences links encountered within a
listed directory or follows them during `-R` traversal.

A confirmed Bashy-owned gap has been fixed: command-line symbolic links pointing to directories were not being dereferenced under `-H` / `-L` when `-d` (directory only) was specified, incorrectly displaying the symlink metadata instead of the referent directory information. This is now resolved: the referent directory metadata (such as permissions, size, and type `d`) is correctly retrieved and displayed.

When `-H` or `-L` explicitly requires dereferencing a command-line symbolic link, failure to resolve its referent is diagnosed and produces a non-zero status. Without explicit dereferencing, `-F` reports the command-line link itself rather than following a link to a directory.

Evidence: `TestLsDereferenceCommandLineSymlinks`,
`TestLsDereferenceModeLastOptionWins`, `TestDereferenceDirectoryEntries`.

## 3. `ls` — Sizing, Radix, and Columns

Block sizes for `-s` default to 512-byte blocks when `POSIXLY_CORRECT` is set (as required by Issue 7), but use 1024-byte blocks by default or when `-k` is specified. The GNU-compatible `-h`, `--si`, and `-w` extensions are retained but are not evidence for the POSIX row. Column formatting uses `COLUMNS` when valid and otherwise falls back to 80 columns; it does not query terminal width.

Evidence: `TestSizeBlocksPOSIX512ByteDefault`, `TestBlockSize`, `TestColumnsHonorsColumnsEnv`.

## 4. `stty` — Options and Mutually Exclusive Styles

POSIX `stty` defines exactly two options: `-a` (write all current settings) and `-g` (write settings in an stty-reusable hex format). Standard input must be associated with a terminal. In line with the standard, `-a` and `-g` are mutually exclusive, and settings cannot be passed when either output style is selected.

Evidence: `TestSttyRejectsNonTTY`, `TestSttyRejectsConflictingOutputStylesBeforeTTY`, `TestSttyRejectsSettingsWithOutputStyleBeforeTTY`.

## 5. `stty` — Settings and Control Character Operands

The source inventories the POSIX terminal modes (control, input, output, local, and combination modes), baud rates/speeds, and control character settings, including `min`, `time`, and disabled control characters using `undef`, on its termios-backed targets. The validation phase checks the complete operand sequence before modification. On an application failure it attempts to restore the original terminal state; focused PTY tests show that invalid later operands do not leave partial changes on the exercised host platform.

Evidence: `TestSttyRowsColsRejectsOverflow`, `TestSttyRowsAppliesWindowSize`, `TestSttyColumnsAppliesWindowSize`, `TestSttyRequiredReportsPropagateWriteErrors`, `TestApplyTermiosModeRaw`, `TestApplyTermiosValueMinTime`.

## 6. Exit Status

Observed exit statuses use the POSIX success/non-success partition:
- `ls` exits `0` on success and `>0` (specifically `1` or `2`) on directory open, stat, or operand errors.
- `stty` exits `0` on successful query/modification and `>0` (specifically `1`) on non-tty stdin, conflicting options, or execution failures.
`stty` propagates standard-output write errors. `ls` output writes are not consistently checked, which remains a partial-profile residual.

Evidence: `TestNonexistentOperand`, `TestUnknownFlag`, `TestSttyRejectsNonTTY`, `TestSttyRequiredReportsPropagateWriteErrors`.

## 7. Platform Disposition and Residuals

Both implementations are pure Go. The residual issues that keep these rows `partial`:
- **Locale Constraints:** `LC_COLLATE` sorting for non-C locales is absent. Non-C `LC_TIME`/`LC_MESSAGES` rendering is unsupported. Missing localized message catalogs are treated as a product localization gap.
- **Terminal Capabilities:** `ls` uses its `-w` extension, then `COLUMNS`, then a fixed width of 80; it does not discover PTY/tty width. `stty` necessarily depends on terminal/PTY facilities.
- **Platform Limitations:** `stty` has termios implementations for Linux, Darwin, FreeBSD, NetBSD, and OpenBSD. Windows, AIX, DragonFly, Solaris, and other unmatched targets use fail-closed stubs; the cross-platform vet commands below are compile-time evidence, not runtime conformance evidence. On Windows, `ls` reports inode 0 and link count 1, while its block count is derived from apparent size; owner/group name lookup is best-effort.
- **Dangling symlink metadata:** Explicit `-H`/`-L` dereferencing failures for command-line operands are diagnosed with non-zero status. Unresolvable symlinks encountered inside a directory under `ls -lL` are diagnosed and use link metadata as a display fallback.
- **Output errors:** `ls` does not consistently propagate failures from standard-output writes.

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
