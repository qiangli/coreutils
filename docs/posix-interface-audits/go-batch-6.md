# POSIX Interface Audit: Go Batch 6

This audit covers exactly 13 Go-owned commands against POSIX.1-2016 (Issue 7): `tput`, `tr`, `tsort`, `tty`, `uname`, `unexpand`, `uniq`, `uudecode`, `uuencode`, `wc`, `who`, `write`, and `xargs`. A passing package test means the implementation's current behavior is internally consistent; it is not, by itself, evidence of POSIX conformance.

## 1. `tput`

**POSIX specification:** [Issue 7/2016: tput](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tput.html)
**Applicability:** User Portability Utilities (`UP`)
**Status:** `implementation_gap`

### Interface definition

- **Synopsis:** `tput [-T type] operand...`
- **Options:** `-T type` overrides `TERM`.
- **Operands:** POSIX requires `clear`, `init`, and `reset` in the POSIX locale. It does not specify the terminfo `capname [parameter...]` interface provided as an extension by this implementation.
- **Environment:** `TERM` and the standard locale variables.
- **Exit:** 0 means the requested string was written; 1 is unspecified; 2 is usage error; 3 means no terminal information; 4 means an invalid operand; greater than 4 means another error. An unavailable operation is not an error, and processing must continue with later operands.

### Disposition

- **Source:** [`cmds/tput/tput.go`](../../cmds/tput/tput.go) processes only `operands[0]` and treats all later operands as parameters for that one terminfo capability. Thus a conforming sequence such as `tput clear reset` is not executed as two operations.
- **Tests:** [`cmds/tput/tput_test.go`](../../cmds/tput/tput_test.go) covers the published terminfo extension and exit statuses, but has no POSIX multi-operation/continue-after-unavailable test.
- **Required work:** Preserve the published terminfo capability behavior while adding an unambiguous path for sequential POSIX `clear`, `init`, and `reset` operands.

## 2. `tr`

**POSIX specification:** [Issue 7/2016: tr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html)
**Applicability:** Base
**Status:** `implementation_gap`

### Interface definition

- **Synopses:** `tr [-c|-C] [-s] string1 string2`; `tr -s [-c|-C] string1`; `tr -d [-c|-C] string1`; `tr -ds [-c|-C] string1 string2`.
- **Options:** `-c`, `-C`, `-d`, and `-s`. GNU/BSD `-t` is an extension, not a POSIX option.
- **Operands/input:** The strings define character arrays; standard input supplies the characters to translate, delete, or squeeze.
- **Environment:** `LC_CTYPE` and `LC_COLLATE` affect character interpretation, classes, ranges, complements, and equivalence classes.

### Disposition

- **Source:** [`cmds/tr/tr.go`](../../cmds/tr/tr.go) expands byte arrays, byte ranges, and one-byte equivalence members. [`cmds/tr/ctype.go`](../../cmds/tr/ctype.go) adds provider-backed class tables but does not make the whole engine multibyte- or collation-aware.
- **Tests:** [`cmds/tr/tr_test.go`](../../cmds/tr/tr_test.go) and [`cmds/tr/ctype_test.go`](../../cmds/tr/ctype_test.go) provide strong C/POSIX and provider-table coverage, but do not establish full multibyte `LC_CTYPE`/`LC_COLLATE` semantics.
- **Required work:** Close only the locale/multibyte/collation gap. POSIX explicitly does not require `[.c.]` collating-symbol syntax for `tr`, so its absence is not a gap.

## 3. `tsort`

**POSIX specification:** [Issue 7/2016: tsort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html)
**Applicability:** Base
**Status:** `verified`

### Interface definition

- **Synopsis:** `tsort [file]`; there are no POSIX options.
- **Input:** A text file, or standard input when omitted (and for `-` as the implementation-defined standard-input spelling), containing blank-separated pairs of non-empty items.
- **Effects/exit:** Write an order consistent with the input. Diagnose cycles, continue sufficiently to produce an order where possible, and return greater than zero for an error.

### Disposition

- **Source:** [`cmds/tsort/tsort.go`](../../cmds/tsort/tsort.go) enforces one operand, handles identical-item presence, diagnoses and breaks cycles, continues sorting, and reports a nonzero status.
- **Tests:** [`cmds/tsort/tsort_test.go`](../../cmds/tsort/tsort_test.go) covers stdin/file input, identical items, odd tokens, cycles, continuation, and the POSIX example. The accepted `-w` behavior is a non-Issue-7 extension and is not relied on for this disposition.

## 4. `tty`

**POSIX specification:** [Issue 7/2016: tty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tty.html)
**Applicability:** Base
**Status:** `verified`

### Interface definition

- **Synopsis:** `tty`; POSIX Issue 7 specifies no options and no operands.
- **Input/effects:** Examine standard input without reading it. Write the equivalent of `ttyname()` when it is a terminal, otherwise write an informative message.
- **Exit:** 0 when standard input is a terminal, 1 when it is not, and greater than 1 for an error.

### Disposition

- **Source:** [`cmds/tty/tty.go`](../../cmds/tty/tty.go) implements the terminal check, message, operand rejection, and required exit partition.
- **Tests:** [`cmds/tty/tty_test.go`](../../cmds/tty/tty_test.go) includes a real PTY test, non-terminal cases, operand rejection, and write-error behavior.
- **Extension note:** `-s`, `--silent`, and `--quiet` are implementation extensions. The obsolete POSIX `-s` form was removed before Issue 7 and is not part of the audited interface.

## 5. `uname`

**POSIX specification:** [Issue 7/2016: uname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html)
**Applicability:** Base
**Status:** `implementation_gap`

### Interface definition

- **Synopsis:** `uname [-amnrsv]`; no operands.
- **Options:** `-a` is exactly the behavior of `-mnrsv`; the remaining options select machine, node name, release, system name, and version. With no option, behavior is `-s`.
- **Output/exit:** Write selected implementation-defined system symbols in `s n r v m` order; return 0 on success and greater than zero on error.

### Disposition

- **Source:** [`cmds/uname/uname.go`](../../cmds/uname/uname.go) implements every required selector and the default, but `-a` also appends the GNU `-o` operating-system field. That is not the POSIX `-mnrsv` equivalence.
- **Tests:** [`cmds/uname/uname_test.go`](../../cmds/uname/uname_test.go) validates the current GNU-oriented output order and therefore does not close this POSIX defect.
- **Required work:** In POSIX mode, make `-a` emit only `-mnrsv`; retain `-o` as an explicitly requested extension.

## 6. `unexpand`

**POSIX specification:** [Issue 7/2016: unexpand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unexpand.html)
**Applicability:** Base
**Status:** `implementation_gap`

### Interface definition

- **Synopsis:** `unexpand [-a|-t tablist] [file...]`.
- **Options:** `-a` converts eligible non-leading blank runs; `-t tablist` selects repeating or explicit tab stops and implies all-run processing.
- **Environment:** `LC_CTYPE` controls byte-to-character interpretation, blank characters, and each character's display-column width.
- **Effects:** Preserve text while replacing eligible blanks with the maximum tabs and minimum spaces occupying the same columns.

### Disposition

- **Source:** [`cmds/unexpand/unexpand.go`](../../cmds/unexpand/unexpand.go) has the required parser and tab-stop algorithm, but treats only space/tab as blanks and increments every decoded rune by one column. It therefore cannot honor locale-defined blanks or wide/zero-width characters.
- **Tests:** [`cmds/unexpand/unexpand_test.go`](../../cmds/unexpand/unexpand_test.go) covers tab arithmetic, UTF-8 code-point counting, malformed UTF-8, and backspace, but not `LC_CTYPE` display widths.
- **Required work:** Make blank classification and display-column width follow the effective `LC_CTYPE` locale.

## 7. `uniq`

**POSIX specification:** [Issue 7/2016: uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html)
**Applicability:** Base
**Status:** `implementation_gap`

### Interface definition

- **Synopsis:** `uniq [-c|-d|-u] [-f fields] [-s chars] [input_file [output_file]]`.
- **Options:** Select counts/duplicate/unique groups and ignore an initial number of locale-defined fields or characters.
- **Environment:** `LC_CTYPE` determines characters and blanks; `LC_COLLATE` affects comparisons.

### Disposition

- **Source:** [`cmds/uniq/uniq.go`](../../cmds/uniq/uniq.go) implements the flags and operands, but `skipKey` skips bytes and recognizes only ASCII space/tab. Comparisons are explicitly byte-wise C-locale comparisons.
- **Tests:** [`cmds/uniq/uniq_test.go`](../../cmds/uniq/uniq_test.go) covers the C-locale behavior and operands, not multibyte `-s`, locale blanks, or collation.
- **Required work:** Count characters rather than bytes and honor `LC_CTYPE`/`LC_COLLATE` for field parsing and comparison.

## 8. `uudecode`

**POSIX specification:** [Issue 7/2016: uudecode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uudecode.html)
**Applicability:** Base
**Status:** `implementation_gap` (fix pending review/integration)

### Interface definition

- **Synopsis:** `uudecode [-o outfile] [file]`.
- **Effects:** Decode historical or Base64 input, use the header pathname unless overridden, and set the produced file's header permissions independently of the process umask. A conforming header can express permissions in `chmod` octal or symbolic notation.
- **Existing files:** Refuse an existing file the user cannot write; when a writable existing file cannot have its mode changed, that mode failure is not itself fatal.

### Disposition

- **Source:** [`cmds/uudecode/uudecode.go`](../../cmds/uudecode/uudecode.go) supports both payload formats and atomic output, but restricts header paths to basenames, accepts only octal modes, masks modes to `0666` (dropping execute bits), can replace an existing non-writable file via directory rename permission, and treats every `Chmod` failure as fatal.
- **Tests:** [`cmds/uudecode/uudecode_test.go`](../../cmds/uudecode/uudecode_test.go) intentionally verifies the current path restriction and does not cover all normative mode/existing-file cases.
- **Required work:** Pending issue 55 must be reviewed and integrated before this disposition can change; bypassing umask is required and is not itself a defect.

## 9. `uuencode`

**POSIX specification:** [Issue 7/2016: uuencode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uuencode.html)
**Applicability:** Base
**Status:** `implementation_gap` (fix pending review/integration)

### Interface definition

- **Synopsis:** `uuencode [-m] [file] decode_pathname`.
- **Options/effects:** `-m` selects the required MIME Base64 encoding; without it, use the historical encoding. The header records the input file permissions and decode pathname.

### Disposition

- **Source:** [`cmds/uuencode/uuencode.go`](../../cmds/uuencode/uuencode.go) implements only historical encoding and deliberately rejects `-m`.
- **Tests:** [`cmds/uuencode/uuencode_test.go`](../../cmds/uuencode/uuencode_test.go) covers historical output but not required Base64 output.
- **Required work:** Pending issue 56 must be reviewed and integrated before this command can be marked verified.

## 10. `wc`

**POSIX specification:** [Issue 7/2016: wc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wc.html)
**Applicability:** Base
**Status:** `implementation_gap`

### Interface definition

- **Synopsis:** `wc [-c|-m] [-lw] [file...]`.
- **Options:** Count bytes, characters, newlines, and words. `-c` and `-m` are mutually exclusive.
- **Environment:** `LC_CTYPE` determines characters and white-space characters.

### Disposition

- **Source:** [`cmds/wc/wc.go`](../../cmds/wc/wc.go) implements selection, formatting, files, and totals, but explicitly sets the character count equal to the byte count and uses a fixed ASCII white-space table.
- **Tests:** [`cmds/wc/wc_test.go`](../../cmds/wc/wc_test.go) covers C-locale counts and formatting, not multibyte `-m` or locale white space.
- **Required work:** Decode characters according to `LC_CTYPE` for `-m` and use its white-space classification for `-w`.

## 11. `who`

**POSIX specification:** [Issue 7/2016: who](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/who.html)
**Applicability:** Base, with XSI-shaded forms and options
**Status:** `implementation_gap` (fix pending review/integration)

### Interface definition

- **Base:** `who [-mTu]` plus the implementation-defined database/file behavior.
- **XSI:** Additional `-abdHlprstq` forms and the exact special operands `who am i` and `who am I`.
- **Quick mode:** `-q` lists only logged-in user names and their count; all other options are ignored.

### Disposition

- **Source:** [`cmds/who/who.go`](../../cmds/who/who.go) accepts any two operands as the special form and also accepts an invalid three-operand form. Its `-q` path uses records already selected by other options, so other options are not ignored and non-user records can affect output/counting.
- **Tests:** [`cmds/who/who_test.go`](../../cmds/who/who_test.go) covers session fixtures but does not establish the exact special-operand and `-q` interactions.
- **Required work:** Pending issue 57 must be reviewed and integrated before these gaps can be closed.

## 12. `write`

**POSIX specification:** [Issue 7/2016: write](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/write.html)
**Applicability:** Base
**Status:** `implementation_gap` (fix pending review/integration)

### Interface definition

- **Synopsis:** `write user_name [terminal]`; no options.
- **Connection:** Write the prescribed initial message to the recipient, then alert the sender's terminal twice. When choosing among multiple recipient sessions, tell the sender which terminal was selected.
- **Termination:** Interrupt or end-of-file must send the POSIX-locale `EOT\n` termination message to the recipient before exit.

### Disposition

- **Source:** [`cmds/write/write.go`](../../cmds/write/write.go) selects and writes a recipient terminal, but does not notify the sender about multiple sessions or alert the sender twice, installs no interrupt handling, emits a GNU-shaped banner, and terminates with `EOF\r\n` rather than `EOT\n`.
- **Tests:** [`cmds/write/write_test.go`](../../cmds/write/write_test.go) validates the current banner/`EOF` contract and terminal permissions rather than the missing POSIX interactions.
- **Required work:** Pending issue 57 must be reviewed and integrated before these gaps can be closed.

## 13. `xargs`

**POSIX specification:** [Issue 7/2016: xargs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html)
**Applicability:** Base, with XSI `-I` and `-L`
**Status:** `implementation_gap` (fix pending review/integration)

### Interface definition

- **Synopsis:** `xargs [-ptx] [-E eofstr] [-I replstr|-L number|-n number] [-s size] [utility [argument...]]` with the `-I`/`-L` portions XSI-shaded.
- **Required behavior:** Prompt through the controlling terminal for `-p`; implement exact quoting, logical-line, replacement, size, `{ARG_MAX}-2048`, default-at-least-`{LINE_MAX}`, execution, stop, and 123-127 status rules.
- **Removed options:** Lowercase `-e`, `-i`, and `-l` were removed in Issue 6 and are not Issue 7 certification requirements.

### Disposition

- **Source:** [`cmds/xargs/xargs.go`](../../cmds/xargs/xargs.go) rejects `-p`; does not discard leading unquoted blanks for `-I`; does not join `-L` lines ending in an unquoted blank; performs no mandatory default system-size batching or environment/`ARG_MAX` accounting; and continues after command lookup/start failures. It already accepts extension aliases `-e` and `-i`, so claiming all lowercase aliases are absent would also be false.
- **Tests:** [`cmds/xargs/xargs_test.go`](../../cmds/xargs/xargs_test.go) covers the current parser, batching, and status aggregation but not all missing Issue 7/XSI cases.
- **Required work:** Pending issue 41 must be reviewed and integrated before this command can be marked verified.
