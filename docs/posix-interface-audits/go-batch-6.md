# POSIX Interface Audit: Go Batch 6

This document audits the POSIX compliance of 13 Go-owned commands: `tput`, `tr`, `tsort`, `tty`, `uname`, `unexpand`, `uniq`, `uudecode`, `uuencode`, `wc`, `who`, `write`, `xargs`.

## Gap Ranking by VSC-PCTS TP Impact

1. **`xargs`**: `-p` (interactive prompt) missing. Explicitly rejected due to lack of controlling terminal support; any PCTS tests exercising interactive confirmation will fail.
2. **`uuencode`**: `-m` (Base64 encoding) missing. Explicitly omitted in favor of the `base64` utility; PCTS tests requiring this variant will fail.
3. **`tr`**: `[.c.]` (collating symbol) missing. The parser lacks support for collating symbols, causing related PCTS translation tests to fail.

---

## 1. `tput`
**POSIX Specification:** [Issue 7: tput](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tput.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tput [-T type] operand`
- **Flags and Option Arguments:** `-T type` (terminal type).
- **Operands, Arity, Special Tokens, Stdin:** `clear`, `init`, `reset`, `longname`, or `capname [parm...]`. Stdin is not used.
- **Environment:** `TERM` (default type), `LINES`, `COLUMNS` (override terminfo).
- **Stdout, Stderr, Effects:** Stdout receives capability strings or evaluated values. Stderr receives diagnostics.
- **Diagnostics and Exit Statuses:** 0 (OK or boolean true), 1 (boolean false), 2 (usage error), 3 (no terminal info), 4 (unknown operand), >4 (error).

### Gap Analysis
- **Parser/Source:** `cmds/tput/tput.go` implements `-T` and maps POSIX operands (`clear`, `init`, etc.) to terminfo capabilities.
- **Behavioral Tests:** `cmds/tput/tput_test.go` verifies operands, missing capabilities, exit statuses, and environment overrides.

---

## 2. `tr`
**POSIX Specification:** [Issue 7: tr](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tr.html)
**Status:** `implementation_gap`
**Missing Feature:** Support for collating symbols (`[.x.]`).

### Interface Definition
- **Synopsis:** `tr [-c|-C] [-s] [-d] [-t] [string1 [string2]]`
- **Flags and Option Arguments:** `-c`, `-C` (complement), `-s` (squeeze), `-d` (delete), `-t` (truncate).
- **Operands, Arity, Special Tokens, Stdin:** `string1`, `string2`. Supports character classes, equivalence classes, octal sequences, and ranges. Stdin is the input.
- **Environment:** `LC_CTYPE`, `LC_COLLATE`.
- **Stdout, Stderr, Effects:** Stdout emits the translated/deleted byte stream. Stderr for diagnostics.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/tr/tr.go` (`parseSetWithTables`) handles standard flags, `[:class:]` and `[=c=]`, but entirely lacks `[.c.]` parsing logic.
- **Behavioral Tests:** `cmds/tr/tr_test.go` exercises `-c`, `-s`, `-d`, `-t`, and equivalence classes but no collating symbols.

---

## 3. `tsort`
**POSIX Specification:** [Issue 7: tsort](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tsort.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tsort [file]`
- **Flags and Option Arguments:** None required by POSIX (Go implementation adds an optional extension).
- **Operands, Arity, Special Tokens, Stdin:** `file` (a text file containing pairs). `-` or empty implies stdin.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives the topological sort order. Stderr receives cycle warnings/diagnostics.
- **Diagnostics and Exit Statuses:** 0 (successful completion), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/tsort/tsort.go` correctly enforces the single operand limit.
- **Behavioral Tests:** `cmds/tsort/tsort_test.go` covers cycles, file vs stdin reading, and expected POSIX outputs.

---

## 4. `tty`
**POSIX Specification:** [Issue 7: tty](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tty.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tty`
- **Flags and Option Arguments:** None.
- **Operands, Arity, Special Tokens, Stdin:** No operands. Stdin is evaluated to see if it is a terminal.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives the terminal name (or "not a tty"). Stderr for diagnostics.
- **Diagnostics and Exit Statuses:** 0 (is a terminal), 1 (not a terminal), 2 (invalid options), 3 (write error), >3 (other error).

### Gap Analysis
- **Parser/Source:** `cmds/tty/tty.go` strictly denies operands and returns POSIX exit codes based on `isatty`.
- **Behavioral Tests:** `cmds/tty/tty_test.go` tests standard devices, files, pipes, and explicit exit codes 0, 1, 2, and 3.

---

## 5. `uname`
**POSIX Specification:** [Issue 7: uname](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uname.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `uname [-amnrsv]`
- **Flags and Option Arguments:** `-a` (all), `-m` (machine), `-n` (nodename), `-r` (release), `-s` (sysname), `-v` (version).
- **Operands, Arity, Special Tokens, Stdin:** None.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout emits the requested system information.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/uname/uname.go` implements all POSIX flags and orders the output fields correctly.
- **Behavioral Tests:** `cmds/uname/uname_test.go` covers `-amnrsv` combinations and default behavior.

---

## 6. `unexpand`
**POSIX Specification:** [Issue 7: unexpand](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/unexpand.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `unexpand [-a|-t tablist] [file...]`
- **Flags and Option Arguments:** `-a` (all blanks), `-t tablist` (comma/space separated tab stops).
- **Operands, Arity, Special Tokens, Stdin:** `file...` input files. `-` implies stdin.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout emits text with spaces converted to tabs.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/unexpand/unexpand.go` implements `-a` and `-t`, parsing the strict ascending decimal list.
- **Behavioral Tests:** `cmds/unexpand/unexpand_test.go` validates tab lists, whitespace collapsing, and UTF-8 handling.

---

## 7. `uniq`
**POSIX Specification:** [Issue 7: uniq](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uniq.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `uniq [-c|-d|-u] [-f fields] [-s char] [input_file [output_file]]`
- **Flags and Option Arguments:** `-c` (count), `-d` (duplicate only), `-u` (unique only), `-f fields` (skip fields), `-s char` (skip chars).
- **Operands, Arity, Special Tokens, Stdin:** Up to two operands: input file and output file. `-` for stdin/stdout.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Output written to stdout or the designated output file.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/uniq/uniq.go` parses all POSIX flags and handles up to 2 operands correctly.
- **Behavioral Tests:** `cmds/uniq/uniq_test.go` covers field/char skipping, input/output files (`TestUniqOperands`), and error cases.

---

## 8. `uudecode`
**POSIX Specification:** [Issue 7: uudecode](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uudecode.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `uudecode [-o outfile] [file]`
- **Flags and Option Arguments:** `-o outfile` (output file path).
- **Operands, Arity, Special Tokens, Stdin:** `file` to decode. `-` implies stdin.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Extracts to file specified in the header or overridden by `-o`.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/uudecode/uudecode.go` implements `-o` override and base64/historical decoding modes.
- **Behavioral Tests:** `cmds/uudecode/uudecode_test.go` tests standard/base64 bodies and header scanning.

---

## 9. `uuencode`
**POSIX Specification:** [Issue 7: uuencode](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uuencode.html)
**Status:** `implementation_gap`
**Missing Feature:** `-m` Base64 encoding.

### Interface Definition
- **Synopsis:** `uuencode [-m] [file] decode_pathname`
- **Flags and Option Arguments:** `-m` (encode using Base64).
- **Operands, Arity, Special Tokens, Stdin:** `file` (optional, defaults to stdin) and `decode_pathname` (required).
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout emits the encoded payload with the header.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/uuencode/uuencode.go` deliberately omits `-m`, documenting it as unsupported.
- **Behavioral Tests:** `cmds/uuencode/uuencode_test.go` verifies the usage error when invalid modes or flags are passed.

---

## 10. `wc`
**POSIX Specification:** [Issue 7: wc](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/wc.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `wc [-c|-m] [-lw] [file...]`
- **Flags and Option Arguments:** `-c` (bytes), `-m` (chars), `-l` (lines), `-w` (words).
- **Operands, Arity, Special Tokens, Stdin:** `file...` to process. Stdin if empty or `-`.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives counts and a total row if multiple files are provided.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/wc/wc.go` implements `-c`, `-m`, `-l`, `-w` and GNU extensions, conforming to the C-locale specification.
- **Behavioral Tests:** `cmds/wc/wc_test.go` tests block sizes, standard flags, and the total lines behavior.

---

## 11. `who`
**POSIX Specification:** [Issue 7: who](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/who.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `who [-mTu] [-abdHlprqt] [file]` or `who -q [file]` or `who am i`
- **Flags and Option Arguments:** `-m`, `-T`, `-u`, `-a`, `-b`, `-d`, `-H`, `-l`, `-p`, `-q`, `-r`, `-t`.
- **Operands, Arity, Special Tokens, Stdin:** `file` or `am i` / `am I`. Stdin not used.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives formatted utmp information.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/who/who.go` parses all flags and uses `parseOperands` to map 2 operands to the `sameHost` behavior (`who am i`).
- **Behavioral Tests:** `cmds/who/who_test.go` validates time formats, operands (`TestWhoAmI`, `TestWhoOperands`), and extensions.

---

## 12. `write`
**POSIX Specification:** [Issue 7: write](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/write.html)
**Status:** `verified`

### Interface Definition
- **Synopsis:** `write user_name [terminal]`
- **Flags and Option Arguments:** None.
- **Operands, Arity, Special Tokens, Stdin:** `user_name` (required), `terminal` (optional). Stdin is the message.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes message directly to another user's terminal line. Stderr for diagnostics.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** `cmds/write/write.go` expects exactly 1 or 2 operands and processes stdin interactively.
- **Behavioral Tests:** `cmds/write/write_test.go` checks terminal selection, denied terminals, control char escaping, and proper delivery.

---

## 13. `xargs`
**POSIX Specification:** [Issue 7: xargs](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/xargs.html)
**Status:** `implementation_gap`
**Missing Feature:** `-p` interactive prompt.

### Interface Definition
- **Synopsis:** `xargs [-ptx] [-E eofstr] [-I replstr] [-L number] [-n number [-s size]] [utility [argument...]]`
- **Flags and Option Arguments:** `-p` (prompt), `-t` (trace), `-x` (exact), `-E`, `-I`, `-L`, `-n`, `-s`.
- **Operands, Arity, Special Tokens, Stdin:** `utility` and `argument...`. Stdin is parsed into arguments.
- **Environment:** `PATH` for resolution, standard locale variables.
- **Stdout, Stderr, Effects:** Executes utility. Stdout/stderr pass through or trace out.
- **Diagnostics and Exit Statuses:** 0 (all success), 126 (found but not executable), 127 (not found), 1-125 (child error).

### Gap Analysis
- **Parser/Source:** `cmds/xargs/xargs.go` correctly implements option clusters and size limits, but immediately errors on `-p` since it lacks a controlling terminal implementation.
- **Behavioral Tests:** `cmds/xargs/xargs_test.go` covers quoting, replacing, tracing, limits, and explicit status forwarding, confirming the `-p` usage error.
