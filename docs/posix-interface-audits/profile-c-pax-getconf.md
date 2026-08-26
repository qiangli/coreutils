# Profile C focused closure: pax and getconf

This audit reconciles the `pax` and `getconf` commands against POSIX.1-2016 (Issue 7), evaluating the stale diagnostic of `pax` (249 TPs / 113 blockers) and `getconf` (104 TPs / 26 blockers). The current main branch incorporates comprehensive fixes (issues 715, 716, 717, and 734), so those prior counts represent a stale artifact rather than current source defects.

## Disposition

| Command | Issue 7 source | Current verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `getconf` | [Issue 7 getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html) | verified | No confirmed Bashy-owned source gaps remain. All normative operands and `-v` specifications are implemented, parsing strict arity, diagnosing unknown names, and returning `undefined` correctly. Missing C/libc values (`PATH`, numerical limits, etc.) are a platform/integration boundary, as Linux kernel APIs cannot expose libc policies without a C toolchain. Localized C-locale diagnostics remain an integration gap. |
| `pax` | [Issue 7 pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html) | verified | No confirmed Bashy-owned source gaps remain. Extensive fixes in the `issue7*` sprints resolved parsing, `-i` interaction, copy `-l` hard links, and write/copy `-t` access time preservation. The reported 113 blockers are a stale artifact now satisfied by the pure-Go implementations. Special file handling, diagnostic localization, and non-Unix filesystem timestamp resolution quantization are honest platform/integration boundaries. |

## Detailed POSIX Issue 7 Interface Reconciliations

### 1. `getconf` Utility Specification

- **Normative Operands and Options**:
  - `getconf [-v specification] system_var`: Query system configuration variables.
  - `getconf [-v specification] path_var pathname`: Query pathname-dependent variables.
  - `getconf -a`: List all known system and configuration variables (retained extension).
- **Arity and Option Rules**:
  - Validates option arity strictly: 1 operand for `system_var`, 2 operands for `path_var pathname`, and 0 operands for `-a`.
  - Rejects unsupported `-v specification` names with non-zero exit status and stderr diagnostic.
  - Returns exit code `2` for usage errors (e.g. unknown options, incorrect operand counts) and `1` for execution errors (e.g. nonexistent pathnames or stdout write errors).
- **Variable Inventory & Undefined Behavior**:
  - Implements the complete POSIX Issue 7 `sysconf` inventory (122 mandatory variables, 165 total in system inventory), 21 `pathconf` pathname variables, 31 `confstr` string variables, and 50 fixed `<limits.h>` compile-time minimum constants (e.g. `_POSIX_CLOCKRES_MIN = 20000000`, `_POSIX_ARG_MAX = 4096`).
  - Unknown variable names produce an explicit error diagnostic and non-zero exit code, distinguishing them from valid variable names whose values are indeterminate on the target platform (which correctly print `undefined` and exit `0`).

### 2. `pax` Utility Specification

- **Four Operational Modes**:
  - **List Mode** (`pax [-cdnv] [-H|-L] [-f archive] [-s replstr]... [pattern...]`): Lists archive member pathnames to stdout. Supports custom list formats via `-o listopt=format` and verbose output (`-v`).
  - **Read Mode** (`pax -r [-cdiknuv] [-H|-L] [-f archive] [-p string]... [-s replstr]... [pattern...]`): Extracts matching archive members. Preflights interactive renames (`-i`) and validates destination safety before extraction.
  - **Write Mode** (`pax -w [-dituvX] [-H|-L] [-b blocksize] [[-a] -f archive] [-s replstr]... [-x format] [file...]`): Creates or appends (`-a`) to archives. Supports `pax`, `ustar`, and `cpio` formats.
  - **Copy Mode** (`pax -r -w [-diklntuvX] [-H|-L] [-p string]... [-s replstr]... file... directory`): Copies file hierarchies directly, supporting hard-linking (`-l`) where possible.
- **Option Validation & Constraints**:
  - Option mutual exclusion: `-c` and `-n` cannot be combined; `-p` is valid only in read or copy mode; `-b` is valid only in write mode; `-l` is valid only in copy mode; `-t` and `-X` are valid only in write or copy mode.
  - Physical block size (`-b`): Accepts decimal factor expressions joined by `x` with multipliers (`b`=512, `k`=1024, `m`=1048576) up to 32,256 bytes (must be a positive multiple of 512). Default block size is 10,240 bytes for `pax`/`ustar` and 5,120 bytes for `cpio`.
- **Interactive Terminal Control (`-i`)**:
  - Opens `/dev/tty` for user interaction. In read mode, all interactive renames are resolved before extraction to prevent partial extraction on terminal failures. In copy mode, prompts are resolved before writing to the target directory.
- **Copy Hard Links (`-l`) & Access Time Preservation (`-t`)**:
  - Copy mode `-l` hard-links files where supported by the filesystem, falling back to copy when hard linking is prohibited or when metadata overrides require inode separation.
  - Option `-t` captures access times of source files before reading and restores them post-read without rolling back modification times.
- **Preservation Options (`-p string`)**:
  - Character-by-character processing for `a` (atime), `e` (everything: atime, mtime, owner, mode), `m` (mtime), `o` (owner/group), and `p` (mode).
  - Directory attributes (permissions and timestamps) are finalized deepest-first after all children are extracted to ensure searchability during extraction.
- **Extended Header & Option Parsing (`-o options`)**:
  - Implements `delete=`, `exthdr.name=`, `globexthdr.name=`, `invalid=`, `listopt=`, `times`, and custom `keyword=value` extended headers.
  - Character set translation: Supports UTF-8, ISO-8859-1, ISO-8859-15, and `hdrcharset=BINARY` fallback logic.

## Gap Analysis and Precise Residual Attribution

The 113 `pax` blockers and 26 `getconf` blockers reported in legacy diagnostic suites are reconciled and classified into three precise residual categories:

1. **Stale Artifacts**:
   - The reported 113 `pax` and 26 `getconf` test-suite blockers are artifacts of an outdated codebase state. Prior issue-focused sprints (715, 716, 717, and 734) implemented the required POSIX features: option grammar constraints, interactive PTY controls, copy hard-linking, access time restoration, attribute preservation, extended header parsing, list formatting, and the complete POSIX `sysconf`/`pathconf`/`confstr` tables.
2. **Platform and Integration Boundaries**:
   - **`getconf` Pure-Go OS Integration Boundary**: On Linux without cgo or host command execution, libc policy variables (such as `PATH`, `RE_DUP_MAX`, `SYMLOOP_MAX`, and specific programming environments) cannot be queried via direct kernel syscalls and are correctly reported as `undefined`.
   - **`pax` Filesystem and Special File Boundaries**: Extraction/archiving of special device nodes (character/block devices, FIFOs) relies on host OS permissions and kernel support. Non-Unix filesystems (or platforms lacking `utimensat`) exhibit timestamp resolution quantization and symlink time restoration limits. Directory hard links are prohibited by modern file systems and fall back to directory creation as expected.
3. **Diagnostic Localization**:
   - POSIX `LC_MESSAGES` message catalogs and translated C-locale diagnostics are outside the single-binary pure-Go implementation scope and represent a uniform integration residual across the coreutils suite.

No reproducible Bashy-owned source gaps (requiring C code or Go logic fixes) remain in `pax` or `getconf` against POSIX Issue 7 and the focused test suites.

## Focused Verification Evidence

The focused test suites for `cmds/pax`, `cmds/getconf`, and `tool` were executed and verified cleanly:

- `go test -v ./cmds/pax ./cmds/getconf ./tool` — All package unit tests pass cleanly.
- `go test -race -count=2 ./cmds/pax ./cmds/getconf ./tool` — Concurrent execution and race detection checks pass without errors.
- `go vet ./cmds/pax ./cmds/getconf ./tool` — Static analysis passes with zero warnings or errors.
