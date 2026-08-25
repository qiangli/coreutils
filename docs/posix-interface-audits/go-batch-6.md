# POSIX Interface Audit: Go Batch 6

This document audits the POSIX.1-2016 (Issue 7) compliance of 13 Go-owned commands: `tput`, `tr`, `tsort`, `tty`, `uname`, `unexpand`, `uniq`, `uudecode`, `uuencode`, `wc`, `who`, `write`, `xargs`.

---

## 1. `tput`
**POSIX Specification:** [Issue 7: tput](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tput.html)
**Applicability:** User Portability Utilities (`[UP]`)
**Status:** `implementation_gap`
**Missing/Defect Feature:** Multi-operand execution defect.

### Interface Definition
- **Synopsis:** `tput [-T type] operand...`
- **Flags and Option Arguments:** `-T type` (Base option to specify terminal type).
- **Operands, Arity, Special Tokens, Stdin:** `operand...` list. Supports `clear`, `init`, and `reset` (required Base operands), or a `capname` capability name. If `capname` requires parameters, they must immediately follow as operands. Stdin is not used.
- **Environment:** `TERM` (default type), `LINES`, `COLUMNS` (override terminfo).
- **Stdout, Stderr, Effects:** Writes capability strings to stdout. Stderr receives diagnostics.
- **Diagnostics and Exit Statuses:** 0 (success/true), 1 (false/absent), 2 (usage error), 3 (no terminal info), 4 (unknown operand), >4 (other error).

### Gap Analysis
- **Parser/Source:** [cmds/tput/tput.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tput/tput.go#L61-L102)
- **Behavioral Tests:** [cmds/tput/tput_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tput/tput_test.go#L51-L100) (specifically `TestExitStatuses`).
- **Defect/Gap Classification:** Multi-operand defect. POSIX specifies that `tput` accepts multiple capability/operation operands sequentially in a single invocation (e.g. `tput clear cols`). However, the Go parser only processes `operands[0]` as the capability and parses all subsequent arguments `operands[1:]` as arguments/parameters for that single capability. Sequential execution of multiple distinct capabilities is unsupported.

---

## 2. `tr`
**POSIX Specification:** [Issue 7: tr](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tr.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tr [-c|-C] [-s] [-d] [string1 [string2]]`
- **Flags and Option Arguments:** `-c`, `-C` (complement), `-s` (squeeze repeats), `-d` (delete).
- **Operands, Arity, Special Tokens, Stdin:** `string1` and `string2` specifying sets of characters. Stdin is the input.
- **Environment:** `LC_CTYPE`, `LC_COLLATE`.
- **Stdout, Stderr, Effects:** Stdout emits the translated/deleted stream. Stderr for diagnostics.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/tr/tr.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tr/tr.go)
- **Behavioral Tests:** [cmds/tr/tr_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tr/tr_test.go) (covering `TestTr` and `TestTrEquivClass`).
- **Compliance Notes:**
  - **No Collating Symbol Gap:** The previous audit claimed a gap for missing collating symbol `[.c.]` support. Under POSIX.1-2016, `tr` is explicitly not required to support collating symbols ("Collating symbols are not supported by tr..."). Hence, this gap was fabricated and has been removed.
  - **Non-POSIX Option:** The `-t` (truncate `string1`) flag implemented in [cmds/tr/tr.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tr/tr.go) is a non-POSIX extension (GNU/BSD compatibility) and is documented as such.

---

## 3. `tsort`
**POSIX Specification:** [Issue 7: tsort](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tsort.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tsort [file]`
- **Flags and Option Arguments:** None.
- **Operands, Arity, Special Tokens, Stdin:** Optional `file` to read pairs from; defaults to stdin if omitted or `-`.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives topologically sorted items. Stderr receives cycle warnings.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/tsort/tsort.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tsort/tsort.go)
- **Behavioral Tests:** [cmds/tsort/tsort_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tsort/tsort_test.go) (covering `TestTsort` and cycles).

---

## 4. `tty`
**POSIX Specification:** [Issue 7: tty](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/tty.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `tty`
- **Flags and Option Arguments:** `-s` (silent/suppress terminal name output). Note: `-s` is classified as an obsolescent option in POSIX.
- **Operands, Arity, Special Tokens, Stdin:** No operands. Stdin is checked if it is a terminal.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Stdout receives the path to the terminal device, or "not a tty" message.
- **Diagnostics and Exit Statuses:** 
  - `0`: Standard input is a terminal.
  - `1`: Standard input is not a terminal.
  - `>1`: An error occurred.

### Gap Analysis
- **Parser/Source:** [cmds/tty/tty.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tty/tty.go)
- **Behavioral Tests:** [cmds/tty/tty_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/tty/tty_test.go) (verifying exits 0 and 1, and option rejection).

---

## 5. `uname`
**POSIX Specification:** [Issue 7: uname](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uname.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `uname [-amnrsv]`
- **Flags and Option Arguments:** `-a` (all), `-m` (machine), `-n` (nodename), `-r` (release), `-s` (sysname), `-v` (version).
- **Operands, Arity, Special Tokens, Stdin:** None.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes requested system name info to stdout.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/uname/uname.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uname/uname.go)
- **Behavioral Tests:** [cmds/uname/uname_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uname/uname_test.go) (covering `TestUname` combinations).

---

## 6. `unexpand`
**POSIX Specification:** [Issue 7: unexpand](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/unexpand.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `unexpand [-a|-t tablist] [file...]`
- **Flags and Option Arguments:** `-a` (all blanks), `-t tablist` (comma/space-separated list of ascending tab stops).
- **Operands, Arity, Special Tokens, Stdin:** `file...` paths to process; `-` or empty implies stdin.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes text with spaces converted to tabs to stdout.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/unexpand/unexpand.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/unexpand/unexpand.go)
- **Behavioral Tests:** [cmds/unexpand/unexpand_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/unexpand/unexpand_test.go) (covering `TestUnexpand` tab calculations).

---

## 7. `uniq`
**POSIX Specification:** [Issue 7: uniq](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uniq.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `uniq [-c|-d|-u] [-f fields] [-s chars] [input_file [output_file]]`
- **Flags and Option Arguments:** `-c` (prepend count), `-d` (duplicate lines only), `-u` (unique lines only), `-f fields` (ignore first N fields), `-s chars` (ignore first N characters).
- **Operands, Arity, Special Tokens, Stdin:** Optional `input_file` and `output_file` (up to 2 operands). `-` for stdin/stdout.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes unique lines to stdout or `output_file`.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/uniq/uniq.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uniq/uniq.go)
- **Behavioral Tests:** [cmds/uniq/uniq_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uniq/uniq_test.go) (covering `TestUniq` and `TestUniqOperands`).

---

## 8. `uudecode`
**POSIX Specification:** [Issue 7: uudecode](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uudecode.html)
**Applicability:** Base
**Status:** `implementation_gap`
**Missing Feature:** Output path restrictions and umask/mode gaps.

### Interface Definition
- **Synopsis:** `uudecode [-o outfile] [file]`
- **Flags and Option Arguments:** `-o outfile` (explicit destination path; override header value).
- **Operands, Arity, Special Tokens, Stdin:** Optional `file` to decode; stdin if omitted or `-`.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Extracts decoded file to header-defined path or overridden `-o` destination.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/uudecode/uudecode.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uudecode/uudecode.go)
- **Behavioral Tests:** [cmds/uudecode/uudecode_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uudecode/uudecode_test.go) (covering standard and base64 decode).
- **Gaps identified:**
  - **Path restriction gap:** POSIX allows relative or absolute paths inside the `begin` line header. However, the Go implementation strictly limits header names to safe relative basenames (`!strings.ContainsAny(name, "/\\")`), failing on any paths with subdirectories or absolute structures.
  - **Mode/Write-Permission gap:** POSIX dictates that the decoded file mode matches the header mode, except that execution bits must be cleared and the file must not be made writable by others unless permitted by umask. The Go implementation uses `mode & 0o666` and updates the permissions via `Chmod` after temporary file creation, which bypasses the host process umask entirely.

---

## 9. `uuencode`
**POSIX Specification:** [Issue 7: uuencode](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/uuencode.html)
**Applicability:** Base
**Status:** `implementation_gap`
**Missing Feature:** `-m` Base64 encoding.

### Interface Definition
- **Synopsis:** `uuencode [-m] [file] decode_pathname`
- **Flags and Option Arguments:** `-m` (encode using Base64 instead of historical format).
- **Operands, Arity, Special Tokens, Stdin:** `file` (optional, defaults to stdin) and `decode_pathname` (required).
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes encoded text with header to stdout.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/uuencode/uuencode.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uuencode/uuencode.go)
- **Behavioral Tests:** [cmds/uuencode/uuencode_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/uuencode/uuencode_test.go) (covering historical format encoding).
- **Gap Classification:** Under POSIX.1-2016, the `-m` (Base64 encoding) option is a required Base feature. The Go implementation deliberately omits `-m` and fails on its usage.

---

## 10. `wc`
**POSIX Specification:** [Issue 7: wc](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/wc.html)
**Applicability:** Base
**Status:** `verified`

### Interface Definition
- **Synopsis:** `wc [-c|-m] [-lw] [file...]`
- **Flags and Option Arguments:** `-c` (bytes), `-m` (characters), `-l` (newlines), `-w` (words).
- **Operands, Arity, Special Tokens, Stdin:** `file...` paths; stdin if empty or `-`.
- **Environment:** `LC_CTYPE`.
- **Stdout, Stderr, Effects:** Emits counts and paths to stdout.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/wc/wc.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/wc/wc.go)
- **Behavioral Tests:** [cmds/wc/wc_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/wc/wc_test.go) (covering `TestWc` standard flags).

---

## 11. `who`
**POSIX Specification:** [Issue 7: who](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/who.html)
**Applicability:** Base, with User Portability Utilities (`[UP]`) options.
**Status:** `implementation_gap`
**Missing Feature:** Operand verification, `-q` flag interaction/standard counting gaps.

### Interface Definition
- **Synopsis:** 
  - `who [-mTu] [-abdHlprt] [file]` (Base, with `[UP]` options `-a`, `-b`, `-d`, `-l`, `-p`, `-r`, `-t`, `-T`, `-u`)
  - `who [-mu] -s [-bHlprt] [file]`
  - `who -q [file]`
  - `who am i` or `who am I`
- **Flags and Option Arguments:** `-q` (quick mode), `-H` (headings), `-m` (stdin-associated session only), other `[UP]` options.
- **Operands, Arity, Special Tokens, Stdin:** Optional `file` for session DB. Special sequence `am i`/`am I` for current session user.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Output list of users/events.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/who/who.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/who/who.go)
- **Behavioral Tests:** [cmds/who/who_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/who/who_test.go) (specifically `TestWhoFileAndCount`).
- **Gaps identified:**
  - **Operand verification gap:** POSIX strictly defines that when two operands are given, they must be `am i` or `am I`. The Go implementation accepts any two-operand list (e.g. `who foo bar`) as a match for the `am i` behavior (setting `sameHost` to true) without verifying the string content.
  - **`-q` Quick option gap:** POSIX specifies that if `-q` is provided, all other options shall be ignored. The Go implementation still applies filtering and counting logic based on other options (e.g., `-b` or `-d`) if combined, rather than ignoring them and listing standard user processes.
  - **Counting gap:** The number of users reported under quick mode `-q` is based on whatever records are filtered, not restricted to the standard logged-in users.

---

## 12. `write`
**POSIX Specification:** [Issue 7: write](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/write.html)
**Applicability:** Base
**Status:** `implementation_gap`
**Missing Feature:** Signal handling, alerts, banner/EOT conformity, and multi-session notification gaps.

### Interface Definition
- **Synopsis:** `write user_name [terminal]`
- **Flags and Option Arguments:** None.
- **Operands, Arity, Special Tokens, Stdin:** `user_name` (required target user), `terminal` (optional terminal device). Stdin is the message stream.
- **Environment:** Standard locale variables.
- **Stdout, Stderr, Effects:** Writes messages directly to recipient's terminal device.
- **Diagnostics and Exit Statuses:** 0 (success), >0 (error).

### Gap Analysis
- **Parser/Source:** [cmds/write/write.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/write/write.go)
- **Behavioral Tests:** [cmds/write/write_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/write/write_test.go) (specifically `TestWrite` and permission checks).
- **Gaps identified:**
  - **SIGINT / Interrupt Signal Gap:** POSIX specifies that if `write` is interrupted by a signal (e.g. SIGINT), it must write a graceful termination message to the recipient's terminal before exiting. The Go implementation does not capture signals and terminates abruptly.
  - **Alerts Gap:** POSIX requires that upon a successful connection, the sender's terminal must be alerted twice (using bell characters `\a\a`) to indicate transmission has started. This is not implemented.
  - **Multi-session Gap:** If a user is logged in more than once and no terminal operand is provided, POSIX allows selecting one in an implementation-defined manner. The Go implementation chooses the most recent login, but does not notify the sender that multiple sessions exist.
  - **Banner / EOT Gap:** Our implementation outputs `EOF\r\n` on stream termination instead of the POSIX-required `EOT` (End of Transmission) marker.

---

## 13. `xargs`
**POSIX Specification:** [Issue 7: xargs](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/xargs.html)
**Applicability:** Base, with obsolescent XSI options.
**Status:** `implementation_gap`
**Missing Feature:** `-p` interactive confirmation (Base gap) and obsolescent XSI options gaps.

### Interface Definition
- **Synopsis:** `xargs [-ptx] [-E eofstr] [-I replstr] [-L number] [-n number [-s size]] [utility [argument...]]`
- **Flags and Option Arguments:**
  - Base: `-p` (prompt/interactive), `-t` (trace), `-x` (exact), `-E eofstr`, `-I replstr`, `-L number`, `-n number`, `-s size`.
  - Obsolescent XSI Extensions: `-e [eofstr]`, `-i [replstr]`, `-l [number]`.
- **Operands, Arity, Special Tokens, Stdin:** `utility` (default `/bin/echo`) and optional arguments. Stdin provides items to batch.
- **Environment:** `PATH`, standard locale variables.
- **Stdout, Stderr, Effects:** Executes utility.
- **Diagnostics and Exit Statuses:** 0 (all success), 126 (found but not executable), 127 (not found), 1-125 (child exit status/error).

### Gap Analysis
- **Parser/Source:** [cmds/xargs/xargs.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/xargs/xargs.go)
- **Behavioral Tests:** [cmds/xargs/xargs_test.go](file:///Users/qiangli/.bashy/weave/coreutils-36223d01/workspaces/issue-42/cmds/xargs/xargs_test.go).
- **Gaps identified:**
  - **Interactive prompt (`-p`) gap:** The Base option `-p` is missing due to a lack of controlling terminal support in the runner.
  - **Obsolescent XSI option gaps:** POSIX requires supporting `-e` (eofstr), `-i` (replstr), and `-l` (number) as obsolescent XSI extensions. The Go implementation does not parse or support these lowercase flags.
