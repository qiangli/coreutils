# POSIX interface audit — Go batch 4

This is the original point-in-time batch audit. Later closure audits are
authoritative for repaired behavior: `nohup` commit `9c85d14` removed the
125/POSIX split and Issue 779 (`bcd6c42`) closed the no-option operand boundary;
pax Issues 715-717,
775, 776, and 778 closed the interaction/preservation/options/block/listing
surfaces; and the later od Profile C audit closed its carried-locale rendering.
The source excerpts and classifications below are retained as historical
pre-fix evidence, not statements about current main.

**Scope (13 commands, exactly):** `mkdir`, `mkfifo`, `more`, `mv`, `newgrp`,
`nice`, `nohup`, `od`, `paste`, `pathchk`, `pax`, `pr`, `ps`.

**Reference:** POSIX.1-2008 / Issue 7, 2016 edition (XCU), per
`docs/reference-policy.md`. Each section links the exact Open Group page.
GNU compatibility is out of scope; GNU/util-linux-spelled extra flags are
recorded as extensions only where they do not collide with a POSIX meaning.

**Method.** For every command: (1) the normative interface was transcribed
from the linked Issue 7 (2016 edition) page — synopsis (required vs
XSI/optional shading), every option and option-argument, operands/arity/
special tokens, STDIN, ENVIRONMENT VARIABLES, STDOUT, STDERR, EXIT STATUS,
CONSEQUENCES OF ERRORS; (2) the actual parser and behavior were read from
`cmds/<name>/*.go` (flag registrations, manual pre-scans, operand handling,
exit paths — never from `--help` text and never inferred from package
existence); (3) each element was matched to a focused behavioral test in the
repo, and (4) suspect areas were probed by executing the freshly built
multicall binary (`go build ./cmd/coreutils`, 2026-08-24, this branch) in
throwaway temp dirs. Probe transcripts for confirmed gaps are quoted inline;
all scratch dirs were deleted and nothing outside this document was changed.

**Classification.** Every interface element gets exactly one state:

- `verified` — source implements the Issue 7 semantics **and** a focused
  behavioral test in this repo exercises it (source path + test
  path#TestName cited).
- `implementation_gap` — the source demonstrably lacks or deviates from the
  required behavior (source path cited, missing feature named, probe
  evidence where runnable).
- `evidence_gap` — the source appears conformant but no focused repo test
  covers the element (the missing test is named). Evidence gaps are never
  upgraded on the strength of an ad-hoc probe alone; the probe result is
  noted, the test still has to land.

Repo-wide documented deviations that recur below and are counted once, not
per command: usage errors exit **2** (POSIX only requires >0; recorded, not
a gap), and `LC_ALL=C` semantics by contract (multibyte/locale-dependent
clauses are noted where POSIX makes them observable, e.g. `od -c`).

A ranked list of all confirmed `implementation_gap` findings by likely
VSC-PCTS TP impact closes the document.

---

## mkdir

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `mkdir [-p] [-m mode] dir...` (all required; no XSI shading).

**Options & option-arguments:** `-m mode` — set the file permission bits of
each newly created directory to *mode*, using the `chmod` utility's mode
grammar; in symbolic modes `+`/`-` are interpreted relative to an assumed
initial mode of `a=rwx`. `-p` — create missing intermediate components;
each intermediate is created as if by `mkdir()` with mode zero followed by
`chmod` with mode `(S_IWUSR|S_IXUSR|~filemask)&0777` (filemask = process
umask); existing directories (including the final one) are not an error.

**Operands / arity / special tokens:** one or more `dir` operands; `--` per
the Utility Syntax Guidelines. Missing operand is a usage error.

**Stdin:** not used. **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`,
`LC_MESSAGES`, `NLSPATH` (locale/messages only). **Stdout:** not used.
**Stderr:** diagnostics only. **Effects:** directories created; `-m`
applies to the newly created (final) directory.

**Diagnostics & exit status:** 0 = all directories created successfully, or
`-p` given and all target directories now exist; >0 = an error occurred.
Consequences of errors: default.

### Bashy implementation (parser & behavior)

`cmds/mkdir/mkdir.go` (package `mkdircmd`), pflag via
`tool.NewFlags`/`tool.Parse`. Flags: `-p/--parents` (`mkdir.go:49`),
`-m/--mode` (`mkdir.go:50`), `-v/--verbose` (`mkdir.go:51`, GNU
extension), `-Z/--context` no-op (`mkdir.go:52-56`, extension). Missing
operand → `tool.UsageError`, exit 2 (`mkdir.go:62-63`). `-m` refused
loudly on Windows (`mkdir.go:68-69`). Mode parsing `parseMode`
(`mkdir.go:93-115`): octal (incl. setuid/setgid/sticky digits) and the
full chmod symbolic grammar (`parseSymbolicMode` `mkdir.go:228-290`,
`apply` `mkdir.go:292-355` — omitted-who clauses masked by the process
umask, per chmod). Symbolic modes apply against the `a=rwx` (0777)
default (`mkdir.go:201`). Non-`-p` create: `os.Mkdir(rc.Path(op), 0o777)`
(`mkdir.go:138`). `-p` path `makeAll` (`mkdir.go:150-188`): per-component
creation; existing dir → success (`mkdir.go:152-154`); intermediates
chmod'd to `(0o777&^umask)|0o300` (`mkdir.go:180`) — exactly the POSIX
formula; TOCTOU re-stat on EEXIST (`mkdir.go:166-173`). `-m` applied only
to a final component actually created (`mkdir.go:131-134,192-206`).
Errors → `errf` (`mkdir.go:371-374`) to stderr, exit 1 (`mkdir.go:83-85`).
Real umask read per platform (`cmds/mkdir/umask_unix.go`; Windows stub
returns 0).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| `dir...` operand(s), creation, per-operand continue | verified | `cmds/mkdir/mkdir.go:80-86,138` | `cmds/mkdir/mkdir_test.go#TestMkdirSimple` | |
| Missing operand → usage error | verified | `cmds/mkdir/mkdir.go:62-63` | `cmds/mkdir/mkdir_test.go#TestMkdirUsageErrors` | exit 2 (documented repo deviation; POSIX requires only >0) |
| `--` end-of-options | evidence_gap | `tool/flags.go:27` (pflag interspersed) | — | Probe `mkdir -- -weird` → exit 0, created; no focused mkdir test. Missing: a `--` case in `TestMkdirSimple` |
| `-m` octal mode (incl. special bits) | verified | `cmds/mkdir/mkdir.go:94-105,192-206` | `cmds/mkdir/mkdir_test.go#TestMkdirMode` | |
| `-m` symbolic mode per chmod grammar | verified | `cmds/mkdir/mkdir.go:107-114,228-290` | `#TestMkdirSymbolicMode`, `#TestMkdirSymbolicModeApply`, `#TestMkdirSymbolicModeInvalid` | `u=rwx,go=rx` → 755; invalid modes rejected exit 2 |
| `-m` symbolic `+`/`-` relative to `a=rwx` | verified | `cmds/mkdir/mkdir.go:199-201` | `#TestMkdirSymbolicModeStartsAtDefault`, `#TestMkdirSymbolicModeSubtractsFromDefault` | `+x` → 777, `a-x` → 666; omitted-who umask masking at `mkdir.go:329-331` |
| `-m` affects final directory only (with `-p`) | verified | `cmds/mkdir/mkdir.go:131-134,149` | `cmds/mkdir/mkdir_test.go#TestMkdirMode` (505 case) | |
| `-p` creates intermediates | verified | `cmds/mkdir/mkdir.go:150-188` | `cmds/mkdir/mkdir_test.go#TestMkdirParents` | |
| `-p` intermediate mode `(S_IWUSR\|S_IXUSR\|~umask)&0777` | verified | `cmds/mkdir/mkdir.go:177-185` | `cmds/mkdir/mkdir_unix_test.go#TestMkdirParentsRetainOwnerWriteAndSearch` | Probe: umask 077 → all components 700 (= formula) |
| `-p` on existing directory → exit 0 | verified | `cmds/mkdir/mkdir.go:152-154` | `cmds/mkdir/mkdir_test.go#TestMkdirExisting` | |
| Stdin not used | verified | no `rc.In` reference in `cmds/mkdir/mkdir.go` | all tests run with empty stdin | |
| Environment (locale vars) | evidence_gap | no env reads (deterministic C-locale per agent contract) | — | Conformant as a POSIX-locale-only implementation; no test pins locale-insensitivity |
| Stdout not used (POSIX scope) | verified | output only via `verbosef` `mkdir.go:376-380` (extension flag) | `cmds/mkdir/mkdir_test.go#TestMkdirSimple` (out=="") | `-v` is a non-POSIX extension |
| Stderr diagnostics only | verified | `cmds/mkdir/mkdir.go:371-374` | `#TestMkdirExisting`, `#TestMkdirMissingParentWithoutP` | |
| Exit 0 success / >0 error | verified | `cmds/mkdir/mkdir.go:83-86` | `#TestMkdirSimple` (0), `#TestMkdirExisting` (1), `#TestMkdirUsageErrors` (2) | |
| `-m` on Windows | implementation_gap | `cmds/mkdir/mkdir.go:68-69` refuses with NotSupported, exit 2 | `cmds/mkdir/mkdir_test.go#TestMkdirMode` (windows branch) | Platform gap, refused loudly per repo contract |

### Confirmed gaps (probe transcripts)

None functional on Unix. Probes all matched POSIX: `mkdir -m u=rwx,g=rx
d1` → mode 757, exit 0 (`o` untouched from the `a=rwx` default, matching
GNU); `umask 077; mkdir -p a/b/c` → a, a/b, a/b/c all 700; `mkdir -p a`
(existing) → exit 0; `mkdir a` (existing, no `-p`) → `mkdir: cannot create
directory 'a': File exists`, exit 1; `mkdir` → exit 2; `mkdir -m 999 z` →
`invalid mode '999'`, exit 2; `mkdir -- -weird` → exit 0, created.

---

## mkfifo

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `mkfifo [-m mode] file...` (all required; no XSI shading).

**Options & option-arguments:** `-m mode` — set the file permission bits
of the newly created FIFO to *mode*; the mode option-argument is the same
as the mode operand defined for the `chmod` utility; in symbolic mode
strings the `+` and `-` operators are interpreted relative to an assumed
initial mode of `a=rw`.

**Operands / arity / special tokens:** one or more `file` operands; `--`
per guidelines. Without `-m`, the FIFO is created as if by `mkfifo()` with
mode `a=rw` (0666) as modified by the process umask.

**Stdin:** not used. **Environment:** `LANG`, `LC_ALL`, `LC_CTYPE`,
`LC_MESSAGES`, `NLSPATH`. **Stdout:** not used. **Stderr:** diagnostics
only. **Effects:** FIFO special files created.

**Diagnostics & exit status:** 0 = all specified FIFO files were created
successfully; >0 = an error occurred. Consequences of errors: default.

### Bashy implementation (parser & behavior)

`cmds/mkfifo/mkfifo.go` (package `mkfifocmd`) + `cmds/mkfifo/mkfifo_unix.go`
(`unix.Mkfifo`) / `cmds/mkfifo/mkfifo_other.go` (loud "not supported"
error on non-Unix). Flags: `-m/--mode` (`mkfifo.go:28`), `-Z/--context`
no-op (`mkfifo.go:29`, extension). Missing operand → hand-rolled GNU-style
message, exit 1 (`mkfifo.go:36-39`) — conformant (>0) though it deviates
from the repo's own usage-exit-2 convention (mkdir uses 2). Default mode
0o666 passed to `mkfifo(2)`, kernel umask applies (`mkfifo.go:41,53`).
`-m`: octal ≤07777 (`mkfifo.go:69-71`) or the chmod symbolic grammar
(`mkfifo.go:105-178`) applied to 0o666 (`mkfifo.go:80-85`); after
creation `os.Chmod` sets the exact mode, defeating umask
(`mkfifo.go:58-62`) — the POSIX `mkfifo()`+`chmod()` shape. Invalid
mode → `mkfifo: invalid mode`, exit 1 (`mkfifo.go:74,79`). Per-operand
failures continue, final status 1 (`mkfifo.go:53-65`). The symbolic
`apply` (`mkfifo.go:180`) deliberately does NOT mask omitted-who clauses
by the process umask (comment `mkfifo.go:81-84`) — where it diverges from
the chmod grammar POSIX incorporates by reference (see gap below).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| `file...` operands, FIFO creation | verified | `cmds/mkfifo/mkfifo.go:52-57`, `cmds/mkfifo/mkfifo_unix.go` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoCreatesFIFO`, `#TestMkfifoMultipleOperands` | `ModeNamedPipe` asserted |
| Continue after per-operand failure, exit >0 | verified | `cmds/mkfifo/mkfifo.go:53-65` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoPartialFailureContinues` | exit 1, later operands still created |
| Missing operand → error | verified | `cmds/mkfifo/mkfifo.go:36-39` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoErrors` | exit 1 (>0, conformant; inconsistent with the repo usage=2 convention) |
| `--` end-of-options | evidence_gap | pflag interspersed (`tool/flags.go:27`) | — | Generic mechanism; no mkfifo-specific test of `mkfifo -- -name` |
| Default mode `a=rw` modified by umask | evidence_gap | `cmds/mkfifo/mkfifo.go:41,53` (0o666 through `mkfifo(2)`) | — | Probe: umask 077 → 600, correct; no repo test pins the mode. Missing: a mode assertion in `TestMkfifoCreatesFIFO` |
| `-m` octal (incl. special bits) | verified | `cmds/mkfifo/mkfifo.go:69-71,218-230,58-62` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoMode`, `#TestMkfifoOctalSpecialBits` | `-m` result exempt from umask via chmod |
| `-m` symbolic, `+`/`-` relative to `a=rw` | verified | `cmds/mkfifo/mkfifo.go:80-85` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoSymbolicMode` | `u=rw,go=` → 600, `+x` → 777, `a+X` → 666 (X correct on non-dir) |
| `-m` symbolic omitted-who umask interaction (chmod rule) | implementation_gap | `cmds/mkfifo/mkfifo.go:80-85` comment + `apply` `mkfifo.go:180-216` has no umask parameter | — | POSIX: mode arg "shall be the same as the mode operand defined for the chmod utility", whose omitted-who clauses shall not override the umask; GNU and BSD mkfifo honor it. `mkdir`'s own parser implements the masking (`cmds/mkdir/mkdir.go:329-331`) — the two engines disagree. Probe below |
| Invalid mode diagnosed | verified | `cmds/mkfifo/mkfifo.go:72-80` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoErrors` | `mkfifo: invalid mode`, exit 1 (mode string not echoed — cosmetic) |
| Stdin not used | verified | no `rc.In` reference | all tests use empty stdin | |
| Environment (locale vars) | evidence_gap | no env reads; deterministic C locale | — | Same posture as mkdir |
| Stdout not used | verified | no writes to `rc.Out` in `run()` | `cmds/mkfifo/mkfifo_test.go#TestMkfifoCreatesFIFO` (out=="") | |
| Stderr diagnostics only | verified | `cmds/mkfifo/mkfifo.go:36-37,54,60,74,79` | `#TestMkfifoErrors`, `#TestMkfifoPartialFailureContinues` | |
| Exit 0 all created / >0 error | verified | `cmds/mkfifo/mkfifo.go:52,65` | `#TestMkfifoCreatesFIFO` (0), `#TestMkfifoPartialFailureContinues` (1) | |
| Non-Unix platforms | implementation_gap | `cmds/mkfifo/mkfifo_other.go` (clear error, exit 1) | `cmds/mkfifo/mkfifo_test.go#TestMkfifoCreatesFIFO` (windows branch) | FIFOs unimplementable on Windows; refused loudly per repo contract |

### Confirmed gaps (probe transcripts)

- Omitted-who symbolic mode vs umask: `(umask 077; ./bin/coreutils mkfifo
  -m =rwx p3)` → mode **777**; `(umask 077; /usr/bin/mkfifo -m =rwx
  p3sys)` → mode **700**. POSIX (chmod grammar by reference: omitted who
  shall not override the file mode creation mask) and both GNU and BSD
  mkfifo give 700. Under umask 022 `-m +w` coincides in both (666), which
  is why the repo suite never catches it.

---

## more

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/more.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `more [-ceisu] [-n number] [-p command] [-t tagstring]
[file...]` — the entire utility is **[UP]-shaded (optional)**. Guideline-
conformant, except `'+'` *may* optionally be recognized as an option
delimiter.

**Options & option-arguments:** `-c` redraw-not-scroll (may be silently
ignored); `-e` exit after last line of last file; `-i` case-insensitive
search patterns; `-s` squeeze consecutive empty lines to one; `-u` treat
backspace as printable, keep trailing CR; `-n number` positive decimal
lines-per-screenful (overrides all other sources); `-p command` execute
more-command(s) each time a screen from a new file is displayed;
`-t tagstring` start at the ctags tag (feature optional; required where a
conforming `ctags` exists).

**Operands / arity / special tokens:** `[file...]`; no operands → stdin;
`-` → stdin at that point in the sequence.

**Stdin:** used when no operands or a `-` operand; interactive commands read
from stderr//dev/tty when stdout is a terminal. **Environment:** `COLUMNS`,
`LINES`, `TERM`, `EDITOR` (`v` command), **`MORE`** (option string processed
before command-line options), plus `LANG`/`LC_ALL`/`LC_COLLATE`/`LC_CTYPE`/
`LC_MESSAGES`/`NLSPATH`. **Stdout:** file contents; when stdout is **not a
terminal**, all input files are copied entirely unmodified except for `-s` —
no other option shall have any effect. **Stderr:** diagnostics plus the
prompt/user-command channel in terminal mode. **Effects (EXTENDED
DESCRIPTION, terminal mode):** screenful = lines−1, line folding, backspace
underline/embolden processing, end-of-file prompt naming the next file, and
the interactive command set (`h`, `[count]f`/ctrl-F, `[count]b`/ctrl-B,
`[count]<space>`/`j`/newline, `k`, `d`/ctrl-D, `s`, `u`/ctrl-U, `g`, `G`,
`r`/ctrl-L, `R`, `m`letter, `'`letter, `''`, `[count]/[!]pattern`,
`?[!]pattern`, `n`, `N`, `:e [file]`, `:n`, `:p`, `:t tagstring`, `v`,
`=`/ctrl-G, `q`/`:q`/`ZZ`).

**Diagnostics & exit status:** 0 success, >0 error; `:n`/`:p` file-access
errors affect the final exit status, `:e` errors do not.

### Bashy implementation (parser & behavior)

`cmds/more/more.go` is an explicitly **non-interactive cat-fallback**
(package doc, `more.go:1-5`) with util-linux-flavored flags; there is no tty
detection anywhere — it always copies input to output. Flags
(`more.go:36-47`): `-s/--squeeze` (implemented squeeze, `more.go:133-142`);
`-n/--lines` int + `--number` (parsed, validated ≥0, `more.go:57-68`, then
unused); `-F/--from-line` (util-linux extension, start line, `more.go:129`);
`-P/--pattern` (util-linux extension, literal substring start,
`more.go:114-128`); accepted no-op booleans `-d -l -e -f -p -u -c`
(`more.go:41-47`). `-NUM` numeric shorthand pre-scanned to `-n=`
(`more.go:48-52`). Operands: zero → `-`; `-` → `rc.In` (`more.go:154-159`).
No environment reads at all (no `MORE`/`LINES`/`COLUMNS`/`TERM`/`EDITOR`).
Critically, **`-p` is registered as a boolean** (util-linux "print-over"),
not POSIX `-p command`, and **`-i` and `-t tagstring` are not registered**
(exit-2 unknown-flag). `+` option delimiter not recognized. Exit paths:
per-file open/read error → diagnostic + exit 1 (`more.go:78-85`); usage → 2.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Non-tty mode: copy all files unmodified | verified | `cmds/more/more.go:74-95,98-143` | `cmds/more/more_test.go#TestMoreReadsStdin`, `#TestMoreConcatenatesFiles` | Copies stdin/files in order, unmodified |
| `-s` squeeze consecutive empty lines | verified | `cmds/more/more.go:36,133-141` | `cmds/more/more_test.go#TestMoreSqueezeAndFromLine` | The one option POSIX requires to work non-tty |
| Operands: none → stdin | verified | `cmds/more/more.go:71-73` | `cmds/more/more_test.go#TestMoreReadsStdin` | |
| Operand `-` → stdin | evidence_gap | `cmds/more/more.go:154-159` | — | No test passes an explicit `-` operand. Missing: `TestMoreDashOperand` |
| Multiple file operands in sequence | verified | `cmds/more/more.go:76-90` | `cmds/more/more_test.go#TestMoreConcatenatesFiles` | |
| `-n number` accepted (no effect non-tty = conformant) | verified | `cmds/more/more.go:37-38,57-68` | `cmds/more/more_test.go#TestMoreAcceptsDisplayOnlyFlags` | POSIX: non-tty → no effect; parse+ignore is conformant |
| `-c`, `-e`, `-u` accepted (no effect non-tty = conformant) | verified | `cmds/more/more.go:41-47` | `cmds/more/more_test.go#TestMoreAcceptsDisplayOnlyFlags` | No-op booleans; conformant only because non-tty is the sole mode |
| `-i` | implementation_gap | not registered (`cmds/more/more.go:35-47` has no `i`) | — | Probe: exit 2 unknown-flag; POSIX requires acceptance |
| `-p command` (option-argument) | implementation_gap | `cmds/more/more.go:45` registers `-p` as *bool* | — | Probe: argument swallowed as operand → "No such file or directory", exit 1 |
| `-t tagstring` | implementation_gap | not registered | — | Probe: exit 2 unknown-flag. The repo ships `ctags` as a POSIX external provider, so the "no conforming ctags → feature optional" escape hatch is weak |
| `+` as option delimiter | verified (optional) | not implemented | — | POSIX "may"; probe treats `+2` as a filename — permitted |
| Terminal (interactive) mode: paging, prompt, screenful sizing, backspace/CR handling | implementation_gap | `cmds/more/more.go:1-5` (declared fallback); no tty check in the file | — | Always cats, even to a terminal; the entire terminal-mode EXTENDED DESCRIPTION is absent |
| Interactive command set (`h f b <space> j k d s u g G r R m ' '' / ? n N :e :n :p :t v = q ZZ`) | implementation_gap | absent (no command loop in `cmds/more/more.go`) | — | None implemented |
| Environment: `MORE`, `LINES`, `COLUMNS`, `TERM`, `EDITOR` | implementation_gap | no `rc.Env` access in `cmds/more/more.go` | — | Probe: `MORE=-s` ignored (output unsqueezed); POSIX requires `$MORE` options be processed even though only `-s` matters non-tty |
| Exit status 0 / >0 | verified | `cmds/more/more.go:78-95` (1), usage → 2 | `cmds/more/more_test.go#TestMoreRejectsBadLineCounts` | Usage=2 is the documented repo deviation, still >0 |
| Extensions (non-POSIX): `-F`, `-P`, `-NUM`, `-d/-l/-f` | verified (extension) | `cmds/more/more.go:39-45,48-52` | `#TestMorePatternStartsAtMatch`, `#TestMorePatternIsLiteral`, `#TestMorePatternNotFound`, `#TestMoreSqueezeAndFromLine`, `#TestMoreAcceptsDisplayOnlyFlags` | util-linux spellings; no POSIX collision except `-p` (gap above) |

### Confirmed gaps (probe transcripts)

- `./bin/coreutils more -i m1` → `more: unknown shorthand flag: 'i' in -i`,
  exit 2. Required: `-i` accepted (no effect when stdout is not a terminal).
- `./bin/coreutils more -p :n m1` → `more: :n: No such file or directory` +
  file contents, exit 1. Required: `-p` takes *command* as its
  option-argument (`:n` must not become an operand).
- `./bin/coreutils more -t tag m1` → `more: unknown shorthand flag: 't' in
  -t`, exit 2. Required: `-t tagstring` parsed.
- `MORE=-s ./bin/coreutils more m1` → blank lines NOT squeezed, exit 0.
  Required: options in `$MORE` processed as if prepended to the command
  line.
- Interactive mode: by source inspection (no tty branch exists), `more file`
  on a terminal cats the whole file with no paging/prompt/commands. The
  whole utility is [UP]-optional, but shipped under the name `more` the
  deviation must be recorded.

---

## mv

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mv.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `mv [-if] source_file target_file` | `mv [-if]
source_file... target_dir` (both required; no XSI shading). The second
form applies when the final operand names an existing directory; ≥3
operands with a non-directory final operand is an error.

**Options & option-arguments:** `-f` — do not prompt for confirmation if
the destination exists; any previous `-i` is ignored. `-i` — prompt for
confirmation if the destination exists; any previous `-f` is ignored;
affirmative response → proceed, otherwise do nothing more with that
source_file and continue with the rest. The last `-i`/`-f` wins.

**Operands / arity / special tokens:** one or more `source_file`; exactly
one `target_file`/`target_dir`; `--` per guidelines.

**Normative description steps:** (1) **prompt even without `-i`** when
the destination exists, `-f` is not specified, and either the
destination's permissions do not permit writing AND stdin is a terminal,
or `-i` was given — prompt written to stderr; (2) same-file: destination
not removed; a diagnostic is permitted, then continue; (3) `rename()`;
on `[EXDEV]` go to the copy steps; (4) dir/non-dir type mismatch →
diagnostic, skip; (5) remove an existing destination that blocks the
copy; (6) cross-filesystem: duplicate the hierarchy (symlinks duplicated
as links, not followed) and duplicate file characteristics: **modification
and access times**, **user and group IDs**, and file mode; if uid/gid/mode
of a regular file cannot be duplicated, the S_ISUID and S_ISGID bits
shall not be duplicated; (7) remove the source hierarchy, diagnostic on
failure.

**Stdin:** a line is read in response to each prompt. **Environment:**
`LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`.
**Stdout:** not used. **Stderr:** prompts (step 1) and diagnostics.
**Effects:** files moved; mv shall not modify both source and destination
simultaneously.

**Diagnostics & exit status:** 0 = all input files moved successfully; >0
= an error occurred.

### Bashy implementation (parser & behavior)

`cmds/mv/mv.go` (package `mvcmd`) + `cmds/mv/xdev_unix.go` (EXDEV),
`xdev_windows.go` (ERROR_NOT_SAME_DEVICE 0x11 or EXDEV), `xdev_other.go`
(always false). Flags (`mv.go:54-67`): `-f/--force`, `-n/--no-clobber`,
`-i/--interactive`, `-u/--update`, `-t/--target-directory`,
`-T/--no-target-directory`, `-b/--backup`, `-S/--suffix`, `--debug`,
`--strip-trailing-slashes`, `-g/--progress` no-op, `-Z/--context` no-op,
`-v/--verbose` — everything beyond `-f`/`-i` is a GNU-or-compat
extension. Arity: 0 operands → exit 2 (`mv.go:82-84`); 1 without `-t` →
exit 2 (`mv.go:85-88`). Target-dir detection via `os.Stat`
(`mv.go:116-117`); multiple sources to a non-dir → exit 1
(`mv.go:122-124`). Last-of-`-f`/`-i`/`-n` wins via a raw argv re-scan
`lastOverride` (`mv.go:98-106,452-480`) that stops at `--` but naively
walks the bytes of any `-xyz` cluster — including attached option-values
(gap below). Move core (`mv.go:169-216`): Lstat source; same-file check
via `os.SameFile` (`mv.go:188-193`) → diagnostic + exit 1; `-i` prompt
`mv: overwrite '%s'? ` on **stderr** (`mv.go:218-226`), affirmative =
y/Y/yes, declined → skip but `failed=true` (`mv.go:194-197`); `os.Rename`
(`mv.go:202`); on cross-device error `copyMove` (`mv.go:310-319`):
recursive `copyNode` (`mv.go:321-395`) — dirs recreated + chmod
(perm|setuid|setgid|sticky) + mtime; symlinks duplicated as links;
everything else opened and byte-copied as a regular file; the source is
`RemoveAll`'d only after a fully successful copy. Prompts read from
`rc.In` only (`mv.go:228-233`). No env reads and no tty detection
anywhere in the package.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Form 1: `mv src dst` rename | verified | `cmds/mv/mv.go:108-117,202-207` | `cmds/mv/mv_test.go#TestMvRenameFile`, `#TestMvDirRename` | |
| Form 2: `mv src... target_dir` | verified | `cmds/mv/mv.go:116-117,156-161` | `#TestMvIntoDir`, `#TestMvTargetDirectoryAndNoTargetDirectory` | dest = dir/base(src) |
| Multiple sources + non-dir final operand → error | verified | `cmds/mv/mv.go:122-124` | `cmds/mv/mv_test.go#TestMvMultipleToNonDir` | exit 1, diagnostic |
| Arity: 0/1 operands → usage error | verified | `cmds/mv/mv.go:82-88` | `cmds/mv/mv_test.go#TestMvUsageErrors` | exit 2 (documented repo deviation; >0 satisfies POSIX) |
| `--` end-of-options | evidence_gap | pflag + `lastOverride`/`normalize` stop at `--` (`cmds/mv/mv.go:419,457`) | — | Probe `mv -- -weird plain` → exit 0, moved; no focused test |
| `-i`: prompt to stderr when destination exists; declined → skip, continue with rest | verified | `cmds/mv/mv.go:194-197,218-226` | `#TestMvBackupSuffixUpdateAndInteractive`, `#TestMvInteractiveRefusalContinuesAndFails` | y/Y/yes affirmative |
| `-i` declined → exit status | implementation_gap | `cmds/mv/mv.go:194-197` sets `failed=true` → exit 1 (`mv.go:163-165`) | `#TestMvInteractiveRefusalContinuesAndFails` *pins the deviation* | POSIX treats a non-affirmative response as the normal skip flow, not an error; GNU and BSD mv exit 0. Probe below |
| `-f`: no prompt when destination exists | verified | `cmds/mv/mv.go:54,100-102` | `cmds/mv/mv_test.go#TestMvNoClobber` (`-n -f` last-wins overwrite) | Probe `-i -f`: no prompt, overwrote, exit 0 |
| Last of `-i`/`-f` wins (semantics) | verified | `cmds/mv/mv.go:98-106,452-480` | `#TestMvNoClobber` (`-n`→`-f` only) | Probes: `-i -f` → silent overwrite; `-f -i` → prompts. No repo test for the `-i`/`-f` pairs themselves |
| Last-wins re-scan vs attached option-values | implementation_gap | `cmds/mv/mv.go:465-477` walks all bytes of any `-x…` token | — | `mv -i -Sf~ src dst` silently cancels `-i` (probe below) — the `f` inside the `-S` value is read as a trailing `-f` |
| Prompt without `-i` (dest exists, not writable, stdin a terminal, no `-f`) | implementation_gap | `cmds/mv/mv.go:184-201` — prompting gated solely on `-i`; no write-permission or terminal check in the package | — | POSIX step 1 requires this prompt. Probe below (pty) |
| Same-file: destination not removed, diagnostic | verified | `cmds/mv/mv.go:188-193` | `#TestMvSameFile`, `#TestMvBackupControlAndSameFile` | diagnostic permitted by POSIX |
| `rename()` first; EXDEV → copy fallback trigger | evidence_gap | `cmds/mv/mv.go:202-214`; `cmds/mv/xdev_unix.go`, `xdev_windows.go`, `xdev_other.go` | `cmds/mv/mv_test.go#TestMvCopyFallback` exercises `copyMove` directly | The fallback logic is verified; a real EXDEV through `move()` is hermetically untestable — errno trigger is source-reviewed only |
| Type mismatch (dir↔non-dir) diagnosed, skipped | verified | `cmds/mv/mv.go:215,330-334` | `#TestMvTargetDirectoryAndNoTargetDirectory`, `#TestMvNoTargetDirectoryTrailingSlashOnExistingDir` | via rename(2) errno |
| Cross-fs copy: symlinks duplicated as links | verified | `cmds/mv/mv.go:353-369` | `cmds/mv/mv_test.go#TestMvCopyFallback` | link target preserved |
| Cross-fs copy: duplicate mtime + **atime** | implementation_gap | `cmds/mv/mv.go:351,392` — `os.Chtimes(dp, time.Time{}, fi.ModTime())`; zero atime = leave-unchanged | — | POSIX step 6 requires duplicating both last-data-modification and last-access times; atime is never duplicated |
| Cross-fs copy: duplicate user/group IDs | implementation_gap | no `Chown` anywhere in `cmds/mv` | — | POSIX step 6 requires attempting uid/gid duplication |
| Cross-fs copy: S_ISUID/S_ISGID not duplicated when uid/gid can't be | implementation_gap | `cmds/mv/mv.go:350,391` chmod copies setuid/setgid unconditionally while ownership is never duplicated | — | Inverts the POSIX safety rule: setuid/setgid bits survive onto a file now owned by the invoking user |
| Cross-fs copy: FIFOs/device nodes recreated as their own type | implementation_gap | `cmds/mv/mv.go:370-393` default branch byte-copies via `os.Open`+`io.Copy` — a writer-less FIFO blocks indefinitely | — | POSIX step 6 duplicates the hierarchy (cp -R semantics recreate FIFOs). Not probed (would hang); source-cited |
| Source removed only after successful copy | verified | `cmds/mv/mv.go:310-319` | `cmds/mv/mv_test.go#TestMvCopyFallback` | No failure-injection test that the source survives a partial copy (noted, not separately counted) |
| Stdin: read one line per prompt only | verified | `cmds/mv/mv.go:218-233` | `#TestMvInteractiveRefusalContinuesAndFails` (consumes `n\ny\n` across two prompts) | |
| Environment (locale vars) | evidence_gap | no env reads; deterministic C locale; affirmative = y/yes | — | Conformant as POSIX-locale-only; untested |
| Stdout not used (POSIX scope) | verified | stdout written only under `-v` (`cmds/mv/mv.go:402-406`, extension) | `cmds/mv/mv_test.go#TestMvRenameFile` (out=="") | |
| Stderr: prompts + diagnostics | verified | `cmds/mv/mv.go:219,397-400` | `#TestMvBackupSuffixUpdateAndInteractive` (prompt in stderr), `#TestMvMissingSource` | |
| Exit 0 all moved / >0 error | verified | `cmds/mv/mv.go:163-166` | `#TestMvRenameFile` (0), `#TestMvMissingSource` (1), `#TestMvUsageErrors` (2) | modulo the declined-prompt row above |

### Confirmed gaps (probe transcripts)

- **No prompt for an unwritable destination on a terminal** — pty probe:
  `echo N2>s2; echo O2>d2; chmod 444 d2; script -q outlog ./bin/coreutils
  mv s2 d2` → no prompt, `d2` silently replaced (content `N2`), source
  removed, exit 0. POSIX step 1 requires a stderr prompt (dest exists +
  not writable + stdin a terminal + no `-f`). GNU/BSD mv prompt here.
  Root cause: `cmds/mv/mv.go:194` gates prompting solely on `-i`.
- **Declined `-i` prompt exits 1** — `printf 'n\n' | ./bin/coreutils mv
  -i a b` → prompt, skip, **exit 1**; `/bin/mv -i` with the same input →
  "not overwritten", **exit 0** (GNU likewise 0). Source:
  `cmds/mv/mv.go:194-197`; pinned in the wrong direction by
  `#TestMvInteractiveRefusalContinuesAndFails`.
- **`lastOverride` misparses attached option-values** — `printf 'n\n' |
  ./bin/coreutils mv -i -Sf~ src dst` → **no prompt**, dst silently
  overwritten, source removed, exit 0. The `f` inside the `-S` value
  `f~` was read as a trailing `-f` (`cmds/mv/mv.go:465-477`), cancelling
  the explicit `-i`.
- Cross-fs characteristic gaps (atime, uid/gid, conditional
  setuid/setgid stripping, FIFO/device recreation) are source-cited
  above; not runtime-probed because a hermetic EXDEV is unavailable (and
  the FIFO case would block).

---

## newgrp

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/newgrp.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `newgrp [-l] [group]`. Guidelines apply "except for the
unspecified usage of `-`" (a first argument of `-` is unspecified).

**Options & option-arguments:** `-l` — change the environment to what
would be expected if the user actually logged in again.

**Operands / arity / special tokens:** `group` — a group name from the
group database or a non-negative numeric group ID; if a numeric string
exists in the group database as a group *name*, the GID associated with
that name shall be used (getgrnam precedence). With no operand, newgrp
shall change the effective group back to the user's user-entry group
**and set the list of supplementary groups to that set in the user's
group database entries**.

**Stdin:** not used; `/dev/tty` is read for the password when one is
required (INPUT FILES). **Environment:** locale vars only. **Stdout:**
not used. **Stderr:** diagnostics and the password prompt. **Effects:** a
new shell execution environment with new real **and** effective GID,
retaining cwd, umask, and exported variables; a failure to assign the new
group identifications shall NOT prevent the new shell from being created.
Supplementary-group adjustment rules: old EGID in the list → change EGID
(add the new EGID if absent and there is room); old EGID not in the
list → delete the new EGID from the list, add the old EGID if room.
Password rules: passworded group + non-member → prompt; member → never
prompted; no password → implementation-defined whether non-members may
change.

**Diagnostics & exit status:** if newgrp succeeds in creating the shell
(whether or not the group change succeeded), exit status = the shell's;
otherwise >0. Consequences of errors: the invoking shell may terminate.

### Bashy implementation (parser & behavior)

`cmds/newgrp/newgrp.go`: `-l/--login` (`newgrp.go:137-138`); max one
operand, else exit 2 (`newgrp.go:143-147`). Group resolution
`resolveTargetGroup` (`newgrp.go:204-252`): name-before-numeric-GID —
exactly the getgrnam precedence; no operand → primary GID from the user
entry (`newgrp.go:205-210`). Authorization (`newgrp.go:93-104` over
`cmds/newgrp/db.go`): member (primary GID, listed member, or
supplementary GID — `db.go:234-242`) → no prompt; passworded non-member →
challenge; locked/empty password → deny; unreadable gshadow → explicit
fatal error (`db.go:144-161`). Password read from **/dev/tty with echo
off**, never `rc.In` (`cmds/newgrp/spawn_unix.go:102-116`); crypt
verification supports MD5/SHA-256/SHA-512(+rounds)/bcrypt
(`cmds/newgrp/crypt.go:39-68`). Denial is **not fatal**: diagnostic, then
the shell starts with the group unchanged (`newgrp.go:157-162,184-193`).
Shell spawn (`spawn_unix.go:33-93`): child process (spec allows a
subprocess); GID applied via `syscall.Credential{Gid, NoSetGroups: true}`
(`spawn_unix.go:57-63`); kernel EPERM (non-setuid build) → retried
without the credential per the POSIX rule (`newgrp.go:185-193`); shell
exit status propagated incl. 128+signal (`spawn_unix.go:77-92`). `-l`:
dash-prefixed argv[0] + chdir home (`newgrp.go:171-182`). Shell choice:
`$SHELL` > passwd-file shell (`cmds/newgrp/passwd_unix.go:20-36`) >
`/bin/sh` (`newgrp.go:257-265`). Group DB: /etc/group parsed directly,
/etc/gshadow for hashes, os/user fallback for directory-service hosts
(`db.go:104-135`). Windows: refuses outright
(`cmds/newgrp/spawn_windows.go:20-22`). Supplementary groups are **never
modified** (`spawn_unix.go:29-32,61`).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| `-l` login environment | implementation_gap | `cmds/newgrp/newgrp.go:171-182`; `cmds/newgrp/spawn_unix.go:41-45` | `cmds/newgrp/newgrp_test.go#TestLoginShellArgv0AndDirectory`, `#TestNonLoginShellArgv0AndDirectory`, `#TestLoginFlagSurvivesARefusedChange` only cover argv0/cwd | `-l` prefixes argv0 with `-` and changes cwd, but passes `rc.Env` unchanged. Optional shell profiles do not guarantee the login-expected environment required by Issue 7 |
| group operand by name | verified | `cmds/newgrp/newgrp.go:213`, `cmds/newgrp/db.go:92-96` | `cmds/newgrp/newgrp_test.go#TestGroupOperandByName` | |
| Numeric operand: group *name* precedence (getgrnam rule) | verified | `cmds/newgrp/newgrp.go:214-220` | `#TestNumericOperandPrefersTheGroupName` | |
| No operand → revert to primary group from user entry | verified | `cmds/newgrp/newgrp.go:205-210` | `#TestNoOperandRevertsToThePrimaryGroup` | |
| No operand → restore supplementary list from group database | implementation_gap | `cmds/newgrp/spawn_unix.go:61` (`NoSetGroups: true`) — no setgroups path exists in the package | — | Required: "shall set the list of supplementary groups to that set in the user's group database entries". Never done |
| Supplementary-group add/delete rules on group change | implementation_gap | `cmds/newgrp/spawn_unix.go:29-32,61` (deliberate: setgroups is privileged) | — | Even in a privileged run where the GID change succeeds, the required list adjustments never happen |
| New real+effective GID assigned to the shell | evidence_gap | `cmds/newgrp/spawn_unix.go:48-64` | seam-level only: `#TestCorrectGroupPasswordPermitsTheChange` asserts `spec.GID` | No test performs a real credential change (needs root); unprivileged builds always take the EPERM-retry path |
| Failure to assign group must not prevent shell creation | verified | `cmds/newgrp/newgrp.go:157-162,184-193` | `#TestRefusedChangeStillStartsTheShellWithTheGroupUnchanged`, `#TestKernelRefusalRetriesWithoutTheCredential`, `#TestWrongGroupPasswordIsDeniedButStillStartsTheShell` | Probe confirmed |
| Password prompt for passworded group + non-member | verified | `cmds/newgrp/newgrp.go:93-104,232-244` | `#TestAuthorize`, `#TestCorrectGroupPasswordPermitsTheChange`, `#TestWrongGroupPasswordIsDeniedButStillStartsTheShell`, `#TestPromptFailureIsDeniedNotAssumed` | |
| Member never prompted | verified | `cmds/newgrp/newgrp.go:93-95` | `#TestMemberOfAPasswordedGroupIsNotChallenged`, `#TestNoPromptWhenTheUserIsAlreadyAMember` | |
| Password read from /dev/tty (INPUT FILES) | evidence_gap | `cmds/newgrp/spawn_unix.go:102-116` | — | Tests replace the `promptPassword` seam; the /dev/tty channel itself is hermetically untestable |
| Password prompt written to stderr | implementation_gap | `cmds/newgrp/spawn_unix.go:109` — prompt written to the **tty**, not `rc.Err` | — | Spec STDERR: the password prompt goes to standard error. Minor, arguably safer, but deviates from the letter |
| Exit status = shell's status when a shell was created | verified | `cmds/newgrp/newgrp.go:184-199`, `spawn_unix.go:77-92` | `#TestRefusedChangeStillStartsTheShellWithTheGroupUnchanged` (status 42) + probe (exit 7) | |
| >0 when no shell created (DB unreadable, shell won't start, usage) | verified | `cmds/newgrp/newgrp.go:150-155,194-198,146` | `#TestUnreadablePasswordDatabaseIsFatal`, `#TestShellThatCannotStartIsAnError`, `#TestUsageErrorsStartNoShell` | usage 2 (documented deviation, within ">0") |
| First argument `-` (unspecified) | verified | lone `-` treated as operand → "no such group: -" + shell | — | Any behavior conforms (spec: unspecified) |
| Shell environment retention (cwd, exported vars, umask) | verified | `cmds/newgrp/spawn_unix.go:38-46` | `#TestNonLoginShellArgv0AndDirectory` | umask inherited by fork naturally |
| Group DB reading (members + passwords, /etc/group + /etc/gshadow + os/user fallback) | verified | `cmds/newgrp/db.go:104-161,166-214` | `cmds/newgrp/db_test.go#TestParseGroupFile`, `#TestGshadowPassword`, `#TestResolvePasswordFromShadow`, `#TestResolvePasswordWithoutAShadowDatabase`, `#TestResolvePasswordWithAnUnreadableShadowDatabase`, `#TestGroupLookupMisses`, `#TestIsMember` | paths injected in tests |
| Extensions: `--login`, `--help/--version` | verified (extension) | `cmds/newgrp/newgrp.go:138`; tool.Parse | `#TestHelpAndVersionStartNoShell` | non-colliding spellings |

### Confirmed gaps (probe transcripts)

- `./bin/coreutils newgrp nonexistent_group_xyz </dev/null; echo $?` →
  `newgrp: no such group: nonexistent_group_xyz`, shell started, exit 0
  (shell's status) — conformant "shell anyway" rule.
- `echo 'id -g; exit 7' | SHELL=/bin/sh ./bin/coreutils newgrp staff` →
  `20`, exit 7 — shell status propagated.
- `./bin/coreutils newgrp staff extra </dev/null` → `newgrp: extra
  operand "extra"`, exit 2 (no shell started; within ">0").
- Supplementary-group manipulation: no probe possible unprivileged; the
  gap is established from source (`cmds/newgrp/spawn_unix.go:61`
  `NoSetGroups: true`, no setgroups call in the package).
- `-l` environment reset: source-established. `newgrp.go:171-182` changes
  only argv0 and cwd, while `spawn_unix.go:41-45` forwards `rc.Env`
  unchanged; the focused tests likewise assert only argv0 and cwd.

---

## nice

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nice.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `nice [-n increment] utility [argument...]` — *utility* is
required by the synopsis.

**Options & option-arguments:** `-n increment` — a positive or negative
decimal integer with the same effect as if the utility had called
`nice()` with that value. Without `-n`: an implementation-defined
increment ≥ 0 relative to the current nice value. Privilege rule: if the
user lacks appropriate privileges to affect the nice value in the
requested manner, nice shall not affect it; a warning may go to stderr,
but this shall not prevent the invocation of the utility or affect the
exit status. Historical `-increment`/`--increment` forms are **not** in
Issue 7.

**Operands / arity / special tokens:** `utility` (special built-ins →
undefined), `argument...`.

**Stdin:** not used. **Environment:** `PATH` (locates the utility) plus
locale vars. **Stdout/Stderr:** not used / diagnostics only.
**Effects:** invokes the utility at the altered nice value.

**Diagnostics & exit status:** if the utility is invoked, nice's exit
status is the utility's; otherwise **1-125** an error occurred in nice,
**126** utility found but not invokable, **127** utility not found.

### Bashy implementation (parser & behavior)

`cmds/nice/nice.go`: hand-rolled first-operand-stopping parser
`parseNice` (`nice.go:48-124`) — `-n VALUE` separate (`nice.go:74-82`),
attached `-nVALUE` (`nice.go:84-86`), long `--adjustment[=]` with
unambiguous-prefix matching (`nice.go:89-112,134-152`), `--` terminator
(`nice.go:64-65`), lone `-` is an operand (`nice.go:115`), the first
non-option argument ends parsing so utility flags are never consumed
(`nice.go:119-120`). Obsolete `-NUM`/`--NUM`/`-+NUM` forms accepted
(`nice.go:69-72,156-165`) — a GNU-documented extension, non-colliding
(a conforming utility name cannot begin with `-`). Default increment 10
(`nice.go:21`). Adjustment clamped to [-39,39] (`nice.go:20-25,57`),
mirroring `nice()`'s own clamping. Bare `nice` prints the current
niceness (`nice.go:40-41`, GNU extension); adjustment without a command →
diagnostic + 125 (`nice.go:36-39`, within POSIX 1-125). Execution
(`nice.go:173-206`): PATH search via `rc.ResolveCommand`; not-found → 127
(`nice.go:176-177,180-183`); EACCES-style start failure → 126
(`nice.go:185-186`); utility status propagated verbatim, signal death →
128+N (`cmds/nice/waitstatus_unix.go:13-19`). Niceness applied via
`unix.Setpriority(PRIO_PROCESS, child_pid, current+adjust)` **after** the
child starts (`nice.go:188`, `cmds/nice/priority_unix.go:15-17`) — never
to nice's own process (in-process host constraint); a setpriority
failure (e.g. EPERM for negative increments) writes a warning and does
not affect invocation or exit status (`nice.go:188-190`). Non-unix:
`setPriority` returns a clear not-supported error
(`cmds/nice/priority_other.go:8-10`) → warning, command still runs.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| `-n increment`, separate & attached, negative & `+`-signed | verified | `cmds/nice/nice.go:74-86` | `cmds/nice/nice_posix_test.go#TestParseNiceOptions` | |
| Default increment (no `-n`): implementation-defined ≥ current | verified | `cmds/nice/nice.go:21,49` (10) | `#TestParseNiceOptions` ("default adjustment") | 10 matches GNU |
| Guidelines: `--` terminator; `-` as operand; options stop at first operand | verified | `cmds/nice/nice.go:64-65,115,119-120` | `cmds/nice/nice_resolve_test.go#TestNiceDoubleDashStopsOptionParsing`, `#TestNiceDoubleDashGuardsCommandStartingWithDash`; `#TestParseNiceDashIsOperand`; `#TestParseNiceOptions` ("command args are not options") | |
| Invalid/missing option-argument, unknown option → status in 1-125 | verified | `cmds/nice/nice.go:52-56,75-78,116-117` (125) | `#TestParseNiceInvalidAdjustment`, `#TestParseNiceMissingArgument`, `#TestParseNiceRejectsUnknownOptions` | 125 = GNU EXIT_CANCELED, inside POSIX 1-125 |
| Utility operand required | verified | `cmds/nice/nice.go:35-42` | `#TestNiceAdjustmentWithoutCommand` (125); `cmds/nice/nice_test.go#TestNicePrintsCurrent` | Bare `nice` prints the current niceness — GNU extension occupying an invocation Issue 7 assigns no behavior to |
| PATH search for the utility | verified | `cmds/nice/nice.go:174` | `#TestResolveCommandResolvesPathEntriesFromRunContext`, `cmds/nice/nice_resolve_test.go#TestNicePathUnsetFindsCommand` | |
| Exit 127 not found | verified | `cmds/nice/nice.go:176-177,180-183` | `cmds/nice/nice_posix_test.go#TestNiceCommandExitStatuses` + probe | |
| Exit 126 found-not-invokable | verified | `cmds/nice/nice.go:185-186` | `#TestNiceCommandExitStatuses` + probe | |
| Utility exit status propagated (incl. 128+signal) | verified | `cmds/nice/nice.go:191-203`, `cmds/nice/waitstatus_unix.go:13-19` | `cmds/nice/nice_resolve_test.go#TestNiceChildExitPropagates`, `cmds/nice/nice_priority_test.go#TestNiceReportsSignalExitCode` | |
| Utility actually runs at the adjusted nice value | implementation_gap | `cmds/nice/nice.go:179,188`; `cmds/nice/priority_unix.go:15-17` | — | The child is started before `setpriority`; it can execute or exit at the old niceness. Issue 7 requires the utility to be invoked at the altered value, so this is a real race, not merely missing evidence |
| Privilege rule: warning to stderr, invocation and exit status unaffected | evidence_gap | `cmds/nice/nice.go:188-190` | — | Probe-confirmed conformant (`nice --10 …` → warning + output + exit 0); no repo test pins it |
| Obsolete `-NUM`/`--NUM`/`-+NUM` forms | verified (extension) | `cmds/nice/nice.go:69-72,156-165` | `#TestParseNiceOptions` (obsolete positive/plus/negative) | Not Issue 7; matches GNU's obsolete forms; non-colliding |
| `--adjustment` long option + prefix abbreviation, `--help/--version` | verified (extension) | `cmds/nice/nice.go:28,89-112` | `#TestParseNiceOptions` (long separate/equals/abbreviation) | |
| Adjustment clamped to [-39,39] rather than rejected | verified | `cmds/nice/nice.go:20-25,57` | `#TestParseNiceOptions` ("adjustment clamped high/low") | Spec delegates semantics to `nice()`, which clamps |
| ENOEXEC fallback (executable text without shebang) | verified | via `rc.StartCommand` | `cmds/nice/nice_exec_unix_test.go#TestNiceRunsExecutableTextWithoutShebang` | |

### Confirmed gaps (probe transcripts)

One conformance-breaking source gap: `rc.StartCommand` starts the child at
`nice.go:179`, and only afterward does `setPriority` run at `nice.go:188`.
The child can execute or exit before adjustment. Other probes:
`nice -n 5 /bin/echo hi` →
`hi`, exit 0; `nice -n 5` → `nice: a command must be given with an
adjustment`, exit 125; `nice -n bogus echo` → exit 125; missing command →
127; mode-644 script → 126; `nice --10 /bin/echo hi` → warning `nice:
cannot set niceness: permission denied` + `hi` + exit 0 (privilege rule
holds). The privilege-warning behavior remains an evidence gap lacking a
focused test.

---

## nohup

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `nohup utility [argument...]`. **Options:** none.

**Operands / arity / special tokens:** `utility` (special built-ins →
undefined), `argument...`.

**Stdin:** not used; if stdin is a terminal, nohup *may* redirect it from
an unspecified file. **Environment:** `HOME` (fallback location for
nohup.out), `PATH`, locale vars. **Stdout:** if not a terminal, the
utility's stdout; otherwise nothing shall be written to stdout.
**Stderr:** if stdout is a terminal, a message shall be written to stderr
naming the file output is being appended to — `nohup.out` or
`$HOME/nohup.out`. **Effects:** at invocation, SIGHUP shall be set to
ignored in the utility; terminal stdout → append to `./nohup.out`
(fallback `$HOME/nohup.out`); if neither can be created/opened for
appending, the utility shall not be invoked; a created file's permission
bits shall be `S_IRUSR|S_IWUSR` (0600); terminal stderr → redirected to
the same open file description as stdout (or to nohup.out when stdout is
a terminal or closed). All other signals: standard actions.

**Diagnostics & exit status:** **126** utility found but not invokable;
**127** an error occurred in the nohup utility OR the utility could not
be found; otherwise the utility's exit status.

### Bashy implementation (parser & behavior)

`cmds/nohup/nohup.go`: no options except exact-literal
`--help`/`--version` matched only as the sole argument
(`nohup.go:21-25`). Missing operand → diagnostic + `internalFailureCode`
(`nohup.go:26-31`): **125 by default, 127 when `POSIXLY_CORRECT` is set**
(`nohup.go:40-45`) — GNU-identical. Terminal detection via
`term.IsTerminal` on Fd-bearing streams (`nohup.go:73-75,228-233`).
Terminal stdin → redirected from /dev/null before command lookup
(`nohup.go:77-86`). Lookup (`nohup.go:163-193`): PATH from RunContext,
distinguishes exists-vs-executable so exec can yield 126; not found → 127
(`nohup.go:88-92`). Output: terminal stdout → `./nohup.out` opened
`O_CREATE|O_WRONLY|O_APPEND, 0o600` with `$HOME/nohup.out` fallback
(`nohup.go:195-210`); open failure → diagnostic + 125/127(POSIXLY_CORRECT),
utility **not invoked** (`nohup.go:100-105`). Terminal stderr → same
writer as stdout (`nohup.go:111-114`). Stderr message: GNU-style
`nohup: ignoring input and appending output to 'nohup.out'` variants
naming the file (`nohup.go:119-142`). SIGHUP: ignored in the utility via
a short-lived `/bin/sh -c "trap '' HUP; exec \"$@\""` wrapper
(`cmds/nohup/nohup_exec_unix.go:15-29` — ignored dispositions persist
across exec; also supplies the ENOEXEC fallback); the nohup invocation
itself is made immune for the duration of the wait via `signal.Notify`
(`cmds/nohup/nohup_hangup_unix.go:26-30`). Exit mapping: utility status
verbatim; ENOENT → 127; other run failure → 126 (`nohup.go:148-160`).
Windows/other: `ignoreHangup` no-op (`nohup_hangup_other.go`); direct
exec (`nohup_exec_windows.go`/`_other.go`).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| No options; operands passed to the utility verbatim | verified | `cmds/nohup/nohup.go:20-31,94` | `cmds/nohup/nohup_test.go#TestNohupRunsCommand`; `cmds/nohup/nohup_exec_unix_test.go#TestNohupExecutableTextFallbackPreservesArgsAndEmptyEnv` | `--help`/`--version` only as sole argument (extension, non-colliding for real use) |
| SIGHUP ignored in the invoked utility | verified | `cmds/nohup/nohup_exec_unix.go:15-29` | `cmds/nohup/nohup_signal_unix_test.go#TestNohupIgnoresHangupForChild` + probe | |
| nohup invocation itself immune while waiting | verified | `cmds/nohup/nohup_hangup_unix.go:26-30`, `nohup.go:145-146` | `#TestNohupInvocationSurvivesHangup` (cites the VSC-PCTS assertion) | needed because this implementation waits rather than exec-overlaying |
| Terminal stdin may be redirected (unspecified file) | verified | `cmds/nohup/nohup.go:73-86` (/dev/null) | `cmds/nohup/nohup_test.go#TestNohupRedirectsTerminalInput` | permitted "may"; matches GNU |
| Terminal stdout → append `./nohup.out` | verified | `cmds/nohup/nohup.go:100-108,195-199` | `cmds/nohup/nohup_test.go#TestNohupRedirectsTerminalOutput` | |
| Fallback to `$HOME/nohup.out` | verified | `cmds/nohup/nohup.go:202-208` | `#TestNohupFallsBackToHomeNohupOut` | |
| Neither file openable → utility not invoked | evidence_gap | `cmds/nohup/nohup.go:100-105` (return before Run) | `#TestNohupDevNullOpenFailure` covers the analogous pre-invocation failure | No focused test of the *double open failure* path specifically. Missing: a both-opens-fail test |
| Created file mode 0600 | evidence_gap | `cmds/nohup/nohup.go:197,204` (0o600) | — | Probe (pty) confirmed `-rw-------`; no test asserts the mode |
| Append semantics (pre-existing nohup.out preserved) | evidence_gap | `cmds/nohup/nohup.go:197,204` (O_APPEND) | — | Redirect tests all start from an empty dir; none seeds existing content |
| Terminal stderr → stdout's open file description / nohup.out | verified | `cmds/nohup/nohup.go:111-114` | `#TestNohupRedirectsTerminalEquivalentOutput`, `#TestNohupRedirectsStderrToStdoutWhenErrIsNil` | |
| Stderr message naming the file when stdout is a terminal | verified | `cmds/nohup/nohup.go:119-142` | `#TestNohupTerminalRedirectionDiagnostics` (all 6 tty combinations, exact strings) + probe | GNU wording includes the required file name |
| Stdout passthrough when not a terminal | verified | `cmds/nohup/nohup.go:99` | `#TestNohupRunsCommand` + probe (no nohup.out created) | |
| PATH search (HOME/PATH from RunContext) | verified | `cmds/nohup/nohup.go:163-193` | `#TestNohupSearchesPATHRelativeToRunContextDir` | |
| Exit 127 utility not found | verified | `cmds/nohup/nohup.go:88-92,155-158` | `#TestNohupNotFoundReturns127` + probe | |
| Exit 126 found-not-invokable | verified | `cmds/nohup/nohup.go:159-160` | `#TestNohupFoundButNotExecutableReturns126` + probe | |
| Utility exit status otherwise | verified | `cmds/nohup/nohup.go:148-153` | `#TestNohupRedirectsTerminalEquivalentOutput` (exit 17) + probe (exit 42) | |
| Exit 127 for an error in nohup itself | implementation_gap (default) | `cmds/nohup/nohup.go:40-45` — default 125; 127 only with `POSIXLY_CORRECT` | `#TestNohupMissing` (125), `#TestNohupMissingPOSIXLYCorrect` (127), `#TestNohupDevNullOpenFailure` (both) | Issue 7 requires 127 unconditionally. Deliberate GNU-identical deviation; certification runs must set `POSIXLY_CORRECT` |
| ENOEXEC fallback + environment preserved across the sh wrapper | verified | `cmds/nohup/nohup_exec_unix.go:19-27` | `#TestNohupExecutableTextFallbackPreservesArgsAndEmptyEnv`, `#TestNohupPreservesInvocationEnvironment` | the `/bin/sh` wrapper is inside the documented spawner exception |

### Confirmed gaps (probe transcripts)

- `./bin/coreutils nohup; echo $?` → `nohup: missing operand` …
  exit 125; `POSIXLY_CORRECT=1 ./bin/coreutils nohup` → same diagnostic,
  exit 127. **Issue 7 requires 127 unconditionally** for nohup-internal
  errors; the default deviates (GNU-compatible, opt-in fix).
- Conformant probes: `nohup sleep 0` piped → exit 0, no nohup.out;
  `nohup sh -c 'kill -HUP $$; echo survived'` → `survived`;
  `nohup missing-cmd` → 127; mode-644 script → 126; `sh -c 'exit 42'` →
  42; under a pty: stderr message naming `nohup.out`, file created
  `-rw-------` (0600) containing the output.

---

## od

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** base (required) `od [-v] [-A address_base] [-j skip]
[-N count] [-t type_string]... [file...]`; **XSI** form
`od [-bcdosx] [file] [[+]offset[.][b]]`. Guideline-conformant *except* the
order of presentation of `-t` (and XSI `-bcdosx`) options is significant.

**Options & option-arguments:** `-A address_base` — one char of `d|o|x|n`;
`-b` [XSI] ≡ `-t o1`; `-c` [XSI] bytes as characters with C escapes
(`\0 \b \f \n \r \t`) else 3-digit octal; `-d` [XSI] ≡ `-t u2`; `-j skip` —
skip bytes over the *concatenated* input; decimal by default, `0x/0X` → hex,
leading `0` → octal; suffix `b|k|m` = ×512/1024/1048576, but if the number
is hex an appended `b` is a final hex digit; combined input shorter than
skip → diagnostic + non-zero exit; `-N count` — format at most count bytes
(same radix rules); fewer available is NOT an error; `-o` [XSI] ≡ `-t o2`;
`-s` [XSI] ≡ `-t d2`; `-t type_string` — types `a` (named IRV chars, low 7
bits only), `c`, and `d|f|o|u|x`; `d/o/u/x` take an optional unsigned byte
count or `C|S|I|L`; `f` takes an optional byte count or `F|D|L`; **multiple
types may be concatenated within one type_string** and multiple `-t` options
are allowed, output lines written per type in the order specified; `-v` —
write all data (without it, output-line groups identical to the previous
group collapse to a single `*` line); `-x` [XSI] ≡ `-t x2`. Multiple
`-bcdostx` may be combined, output in the order specified.

**Operands / arity / special tokens:** `file...`; none → stdin; `-` → stdin
(if the implementation so treats it). XSI offset operand `[+]offset[.][b]`
(octal default, `.` → decimal, `b` → ×512) recognized **only when**: ≤2
operands, **none of `-A -j -N -t -v` specified**, and (the last operand
starts with `+`, or there are two operands and the last starts with a
digit).

**Stdin:** when no operands / `-`. **Environment:** `LANG`, `LC_ALL`,
`LC_CTYPE`, `LC_MESSAGES`, `LC_NUMERIC` (radix char for float output),
`NLSPATH`. **Stdout:** default output as if `-t oS` (offset base unspecified
when no `-A`); blocks = LCM of type sizes (≤16); per block one line per
type in the specified order, fields blank-separated; partial trailing block
null-extended; first line of each block preceded by the cumulative input
offset unless `-A n`; final line = offset of the byte after the last byte
written, no trailing blanks; multibyte printable chars under `c` occupy the
first-byte area with `**` in continuation areas. **Stderr:** diagnostics.
**Effects:** sequential transformed copy of the concatenated inputs.

**Diagnostics & exit status:** 0 all inputs processed successfully; >0
error.

### Bashy implementation (parser & behavior)

`cmds/od/od.go`. Pre-scan `normalizeTypeAliasArgs` rewrites GNU traditional
args `-D -F -H -I -L -O -X -e -f -i -l -s` to `-t <fmt>` (`od.go:199-233`;
note `-s` therefore keeps its XSI `-t d2` meaning and GNU `--strings` lives
on `-S`, `od.go:57,155-164,456-478`). pflag registrations `od.go:51-77`:
`-A/--address-radix` (default `o`, validated to d/o/x/n at
`od.go:166-168`), `-t/--format` StringArray (repeatable), `-N/--read-bytes`,
`-j/--skip-bytes`, `-w/--width` (GNU extension, default 16), `--endian`
(GNU extension), `-S/--strings`, `-a -b -c -d -o -x -v` booleans,
`--traditional`. XSI trailing `+offset` operand handled at `od.go:82-89`
(**unconditionally** — regardless of `-A/-j/-N/-t/-v` or operand count);
`--traditional` additionally accepts a leading `+offset` (`od.go:90-97`);
`parseTraditionalOffset` (`od.go:592-611`): octal default, `.` → decimal,
`b` → ×512, plus 0x hex. The **two-operand numeric-last** XSI form is not
implemented. Format assembly `od.go:98-127`: `-t` strings first, then
boolean type flags **in fixed table order** — command-line order is lost.
`parseFormats` (`od.go:484-496`) splits one `-t` value on comma/space/tab
(extension) but **cannot parse POSIX concatenated type_strings** (`dc`,
`x2d2`); `parseFormat` (`od.go:498-544`) handles one type: sizes 1/2/4/8,
`C/S/I/L` via `sizeAliases` (`od.go:558-567`) — **`F`/`D` for `f` are
absent** (`fL` → f8 works by accident of `L`); bare `d/o/u/x` → 4, `f` → 8
(`od.go:523-535`). `-j`/`-N` via `parseBytes` (`od.go:679-708`):
decimal / 0x hex / leading-0 octal, POSIX `b/k/m` + GNU suffixes; hex
`b`-as-final-digit falls out of `digitInBase` (`od.go:710-718`). Skip
shortfall → `cannot skip past end of combined input`, exit 1
(`od.go:28,270-277,185-190`). Dump loop `od.go:269-322`: 16-byte (`-w`)
blocks, `*` duplicate-group folding unless `-v` (`od.go:294-307`),
cumulative offset starting at skip, trailing offset line unless `-A n`
(`od.go:317-321`). Formatting `od.go:345-425`; `c` per `cChar`
(`od.go:616-639`: `\0 \a \b \t \n \v \f \r`, printable ASCII, 3-digit
octal — byte-wise, no `**` multibyte continuation); `a` per `namedChar`
with the high bit masked (`od.go:642-660`). Inputs: `MultiReader` over
operands, `-` → `rc.In`, open failure → diagnostic + exit 1 + continue
(`od.go:235-267`). Little-endian word interpretation by default (POSIX
leaves byte order implementation-defined). Usage errors exit 2.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Default output ≡ `-t oS`, octal offsets, 16-byte blocks | verified | `cmds/od/od.go:126-127,285,577-586` | `cmds/od/od_test.go#TestODDefaultOctalWords` | `0000000 061141` / `0000002` matches |
| Trailing final-offset line, no trailing blanks; suppressed by `-A n` | verified | `cmds/od/od.go:317-321` | `#TestODDefaultOctalWords`, `#TestODFormatsAndOffsets` | |
| `-A d\|o\|x\|n`; invalid rejected | verified | `cmds/od/od.go:51,166-168,577-586` | `#TestODFormatsAndOffsets` (x, n), `#TestODMultiFormatEndianStringsAndTraditionalSkip` (d) | Hex offsets 6-wide (POSIX leaves format unspecified) |
| `-v` / duplicate-group `*` folding | verified | `cmds/od/od.go:64,294-307` | `cmds/od/od_test.go#TestODDuplicateSuppression` | |
| `-j skip`: decimal/0x hex/leading-0 octal, `b k m` suffixes, hex `b`=digit | verified | `cmds/od/od.go:54,142-145,679-718` | `#TestODByteCountRadixPrefixes`, `#TestODJSkipWithLowercaseSuffix`, `#TestODByteCountLowercaseSuffixes` | Probe: `-j 0x10b` → offset 0000267 (0x10b=267) ✓ |
| `-j` past EOF → diagnostic + non-zero | verified | `cmds/od/od.go:28,270-277,185-190` | `cmds/od/od_test.go#TestODSkipPastEOF` | exit 1 |
| `-N count` radix rules | verified | `cmds/od/od.go:53,134-141,279-281` | `#TestODFormatsAndOffsets` (`-N 3`) | |
| `-N` exceeding input is not an error | evidence_gap | `cmds/od/od.go:279-281` | — | Probe conforms; missing: `TestODReadBytesBeyondEOFNotError` |
| `-t` repeatable; multiple output lines per block, offset only on first | verified | `cmds/od/od.go:52,324-343` | `cmds/od/od_test.go#TestODMultiFormatEndianStringsAndTraditionalSkip` | `-t x2 -t u1` two lines, blank-prefixed second |
| `-t` types `a c d o u x`, sizes 1/2/4/8, `C S` letters | verified | `cmds/od/od.go:498-544,558-567` | `#TestODTypeAliases` (xC,xS), `#TestODNamedCharsVsC`, `#TestODBareTypeDefaultsToIntWidth` | |
| `-t` size letters `I`/`L` | evidence_gap | `cmds/od/od.go:558-567` | — | Implemented, but only word forms (`xint`, `xshort`) are tested; missing focused test for `-t dI`, `-t xL` |
| `-t f` sizes 4/8; bare `f` = double | verified | `cmds/od/od.go:381-388,523-535` | `#TestODShortTypeAliases`, `#TestODBareTypeDefaultsToIntWidth` | |
| `-t f` suffixes `F`/`D` (`L` incidental) | implementation_gap | `cmds/od/od.go:558-567` lacks F/D | — | Probe: `-t fF`/`-t fD` → `unsupported output format`, exit 2. POSIX requires `F|D|L` after `f` |
| Concatenated types in one type_string (`-t dc`, `-t x2d2`) | implementation_gap | `cmds/od/od.go:484-544` parses one type per token | — | Probe: `-t dc` → `unsupported output format: "dc"`, exit 2. POSIX: "Multiple types can be concatenated within the same type_string" |
| Order of `-t`/`-bcdosx` significant | implementation_gap | `cmds/od/od.go:98-125` fixed table order | `cmds/od/od_test.go#TestODShortTypeAliases` *pins the deviation* | Probe: `-c -b` and `-b -c` both emit the o1 line before the c line |
| XSI `-b -c -d -s -x` ≡ o1/c/u2/d2/x2 | verified | `cmds/od/od.go:59-63,199-233` | `#TestODShortAliasesAndWidth` (b,x,d), `#TestODNamedCharsVsC` (c), `#TestODShortTypeAliases` (s) | |
| XSI `-o` ≡ `-t o2` | evidence_gap | `cmds/od/od.go:61` | — | The default o2 output is tested, the `-o` flag itself is not. Missing: `TestODOFlag` |
| Operands: none → stdin | verified | `cmds/od/od.go:235-267` | every `cmds/od/od_test.go` stdin case | |
| Operand `-` → stdin; multiple files concatenated w/ cumulative offset | evidence_gap | `cmds/od/od.go:235-267` | — | No test for a `-` operand or two file operands. Missing: `TestODDashOperand`, `TestODMultipleFilesCumulativeOffset` |
| XSI `+offset[.][b]` trailing operand (radix rules) | verified | `cmds/od/od.go:82-89,592-611` | `#TestODMultiFormatEndianStringsAndTraditionalSkip`, `#TestODTraditionalOffsetRadix` | Octal default, `.` decimal, `b` ×512 correct — but see gating gaps below |
| Offset-operand gating (only when ≤2 operands AND none of `-A -j -N -t -v`) | implementation_gap | `cmds/od/od.go:82` applies whenever the last operand starts with `+`, unconditionally | — | Probe: `od -t o1 f1 +2` consumed `+2` as an offset; POSIX requires it be a filename when `-t` is present |
| XSI two-operand numeric-last offset form (`od file 10`) | implementation_gap | `cmds/od/od.go:82` only matches leading `+` | — | Probe: `od f1 4` → `od: 4: No such file or directory`, exit 1; XSI requires offset interpretation |
| Partial final block null-extended | verified | `cmds/od/od.go:427-450` | `cmds/od/od_test.go#TestODDefaultOctalWords` | 2 bytes → full `061141` word |
| Byte order = memory order (impl-defined) | verified | `cmds/od/od.go:146-153` | `cmds/od/od_test.go#TestODMultiFormatEndianStringsAndTraditionalSkip` | `--endian` is an extension |
| Open failure → diagnostic + exit 1 + continue; exit 0/>0 | verified | `cmds/od/od.go:179-196,254-258` | `#TestODSkipPastEOF` (1), `#TestODRejectsBadFormat` (2) | |
| Extensions (non-POSIX): `-w`, `--endian`, `-S/--strings`, `--traditional`, `-a` flag, GNU traditional aliases, word/comma format aliases | verified (extension) | `cmds/od/od.go:55-57,65,199-233,546-575` | `#TestODShortAliasesAndWidth`, `#TestODMultiFormatEndianStringsAndTraditionalSkip`, `#TestODShortTypeAliases`, `#TestODNewTypeAliases`, `#TestODTraditionalOffsetBeforeFile` | Distinct spellings; `-s` correctly kept as XSI d2, strings moved to `-S` |

`od -c` multibyte note: byte-wise output (no `**` continuation areas, probe
prints `303 251` for `é`) — conformant under the repo's `LC_ALL=C`
contract, nonconformant in multibyte locales; recorded, not counted as a
gap.

### Confirmed gaps (probe transcripts)

- `printf 'AB' | ./bin/coreutils od -t dc` → `od: unsupported output
  format: "dc"`, exit 2. Required: concatenated type_string = two output
  line sets (d then c).
- `printf '\x00\x00\x80\x3f' | ./bin/coreutils od -A n -t fF` (and `fD`) →
  `unsupported output format`, exit 2. Required: `f` followed by `F|D|L`.
- `printf 'A' | ./bin/coreutils od -A n -c -b` → line ` 101` then ` A` —
  identical to `-b -c`. Required: output lines in the order the options
  were specified.
- `./bin/coreutils od -t o1 f1 +2` → dumped from offset 2, exit 0.
  Required: with `-t` specified the offset-operand conditions fail, so `+2`
  is a filename (expected: open error).
- `./bin/coreutils od f1 4` (two operands, numeric last) → `od: 4: No such
  file or directory`, exit 1. Required (XSI): interpret `4` as an octal
  offset operand.

---

## paste

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `paste [-s] [-d list] file...` — no XSI shading; guideline-
conformant.

**Options & option-arguments:** `-d list` — each element one delimiter;
backslash elements: `\n` newline, `\t` tab, `\\` backslash, `\0` empty
string (not NUL; `\0` followed by x/X/digit unspecified; other `\c`
unspecified); the list is used circularly. Without `-s`: the last file's
newlines pass unmodified and the delimiter list resets for each output
line. With `-s`: one output line per input file in command-line order,
newlines (except the file's last) replaced from the list, list reset per
file; **an empty input file yields an output line of only a newline**.

**Operands / arity / special tokens:** `file...` (≥1 per synopsis;
zero-operand behavior unspecified); `-` → stdin, read one line at a time
per `-` instance; implementations must support ≥12 operands.

**Stdin:** only for `-` operands. **Environment:** `LANG`, `LC_ALL`,
`LC_CTYPE`, `LC_MESSAGES`, `NLSPATH`. **Stdout:** joined lines,
newline-terminated; EOF on some-but-not-all files (without `-s`) behaves as
if empty lines were read from the exhausted files. **Stderr:** diagnostics.
**Effects / consequences:** without `-s`, any unopenable input → diagnostic
and **no output to stdout**; with `-s`, default error behavior (continue,
non-zero exit).

**Diagnostics & exit status:** 0 success, >0 error (`paste -d "" …` may
error — permitted).

### Bashy implementation (parser & behavior)

`cmds/paste/paste.go`. Flags (`paste.go:35-38`): `-d/--delimiters` (default
`\t`), `-s/--serial`, `-z/--zero-terminated` (GNU extension). `parseDelims`
(`paste.go:91-133`): `\n \t \\ \0` per POSIX plus `\b \f \r \v` and `\c`→c
(POSIX-unspecified space, GNU-compatible); multibyte delimiter runes; empty
list → `no delimiters specified`, exit 1 (permitted). `delimCycle`
(`paste.go:138-163`): circular, reset per line in parallel mode
(`paste.go:196`) and per file in serial mode (`paste.go:239`); a delimiter
position is consumed for every file including exhausted ones, and the
trailing delimiter is trimmed so the last file's newline passes unmodified
(`paste.go:216-223,263-265`). Parallel mode opens all files first; any
failure → diagnostic, close all, exit 1 with no stdout output
(`paste.go:181-189`). Serial mode: open failure → diagnostic + continue,
exit 1 (`paste.go:232-237`); **empty file → `continue` with no output
line** (`paste.go:257-261`) — deviates from POSIX. Every `-` shares one
buffered stdin so multiple `-` columns interleave (`paste.go:54-61`). Zero
operands default to `-` (`paste.go:48-50`, benign extension). Exit: 0/1;
unknown flag → 2.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Parallel join, newline→tab except last file | verified | `cmds/paste/paste.go:167-226` | `cmds/paste/paste_test.go#TestPasteParallel` ("two files default tab") | |
| EOF-as-empty-lines on unequal inputs (delimiter still consumed) | verified | `cmds/paste/paste.go:198-218` | `#TestPasteParallel` ("first file longer…", "first file shorter…") | `a2\t\n` / `\t2\n` shapes correct |
| `-d list` circular use, reset per line (parallel) | verified | `cmds/paste/paste.go:138-153,196` | `#TestPasteParallel` ("delimiter list cycles and resets per line") | |
| `-d` escapes `\n \t \\` | verified | `cmds/paste/paste.go:109-130` | `#TestPasteParallel` ("escaped tab and newline delimiters") | |
| `-d` `\0` = empty string | verified | `cmds/paste/paste.go:110-111` | `#TestPasteParallel` ("backslash-zero is no delimiter") | |
| `-d ""` → error (permitted) | verified | `cmds/paste/paste.go:92-94,43-46` | `cmds/paste/paste_test.go#TestPasteErrors` | exit 1, diagnostic |
| `-s`: one line per file, newline→delimiter, last newline kept, reset per file | verified | `cmds/paste/paste.go:228-271,239` | `cmds/paste/paste_test.go#TestPasteSerial` (incl. per-file cycle restart) | |
| `-s` + empty input file → output line of only a newline | implementation_gap | `cmds/paste/paste.go:257-261` skips output when nothing was read | `cmds/paste/paste_test.go#TestPasteSerial` *pins the deviation* ("an empty serial input file produces no output") | Probes below; fixing requires updating that test case |
| Operand `-` = stdin, line-at-a-time per instance | verified | `cmds/paste/paste.go:54-61` | `cmds/paste/paste_test.go#TestPasteStdin` ("two `-` operands interleave") | `1\t2\n3\t4\n` |
| ≥12 file operands | evidence_gap | unbounded slice (`cmds/paste/paste.go:172-189`) | — | Probe with 12 columns conforms; missing: a 12-operand case in `TestPasteParallel` |
| Without `-s`, unopenable file → diagnostic, no stdout output | verified | `cmds/paste/paste.go:181-189` | `cmds/paste/paste_test.go#TestPasteErrors` ("parallel missing…out=\"\"") | |
| With `-s`, open failure → continue, exit >0 | verified | `cmds/paste/paste.go:232-237` | `cmds/paste/paste_test.go#TestPasteErrors` ("serial missing…") | |
| Text lines of unlimited length; final line w/o newline handled | verified | `bufio.Reader.ReadBytes` unbounded (`cmds/paste/paste.go:202,242`) | `#TestPasteParallel` ("missing final newline still pastes") | |
| Exit status 0/>0 | verified | `cmds/paste/paste.go:71-85` | `#TestPasteParallel`, `#TestPasteErrors`, `#TestPasteUnknownFlag` | Usage=2 (documented repo deviation, still >0) |
| Extensions (non-POSIX): `-z`, zero-operand→stdin, extra `\b \f \r \v` escapes, multibyte delimiters | verified (extension) | `cmds/paste/paste.go:37,48-50,109-130` | `#TestPasteZeroTerminated`, `#TestPasteStdin`, `#TestPasteParallel` (multibyte cases) | POSIX-unspecified space; GNU-compatible |

### Confirmed gaps (probe transcripts)

- `./bin/coreutils paste -s empty | od -c` (empty = 0-byte file) → **no
  output at all** (`0000000`), exit 0. Required: "If an input file is
  empty, the output line corresponding to that file shall consist of only a
  `<newline>`."
- `./bin/coreutils paste -s empty f3` → `x\ty\tz\n` only, exit 0. Required:
  `\nx\ty\tz\n` (a newline-only line for the empty file, in command-line
  order). `cmds/paste/paste_test.go#TestPasteSerial` currently asserts the
  nonconforming behavior, so the fix must update that test case too.

---

## pathchk

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `pathchk [-p] [-P] pathname...` (guideline-conformant).

**Options & option-arguments:** default (no `-p`) — check each component
of each pathname against the underlying file system; diagnose operands
that: are longer than {PATH_MAX} bytes; contain a component longer than
{NAME_MAX} bytes *in its containing directory*; contain a component in a
directory that is not searchable; or contain a byte sequence not valid
in its containing directory. Non-existent components are not an error if
a matching file could be created without violating the checks. `-p` —
*instead of* the file-system checks, diagnose operands exceeding
{_POSIX_PATH_MAX} (256) bytes, containing a component longer than
{_POSIX_NAME_MAX} (14) bytes, or containing any character outside the
portable filename character set (`A-Z a-z 0-9 . _ -`); the RATIONALE
notes `-p` deliberately does NOT check leading `-` or empty pathnames.
`-P` — diagnose operands containing a component whose first character is
`-`, or that are empty (additive to the default checks, not "instead
of").

**Operands / arity:** `pathname...`, one or more. **Stdin:** not used.
**Environment:** locale vars only. **Stdout:** not used. **Stderr:**
diagnostics only; format unspecified but must indicate the error and the
operand.

**Diagnostics & exit status:** 0 = all operands passed all checks; >0 =
an error occurred.

### Bashy implementation (parser & behavior)

`cmds/pathchk/pathchk.go` (185 lines). pflag `-p/--posix`,
`-P/--posix-special`, plus GNU-style `--portability` (= `-p -P`)
(`pathchk.go:28-30`). Dispatch (`pathchk.go:41-52`): `-p` *replaces* the
default checks; `-P` *adds to* them — matches POSIX/GNU. `checkDefault`
(`pathchk.go:60-83`): empty → diagnostic; path length vs a **hardcoded
per-OS PATH_MAX** (`defaultPathMax` `pathchk.go:85-94` — 260 windows /
1024 BSD+darwin / 4096 else, NUL-inclusive `len(p) >= limit`); component
length vs **hardcoded 255**; existing directory prefixes walked root-down
(`invalidDirectoryPrefix` `pathchk.go:99-131`) checking existence,
dir-ness, and searchability (stat of `prefix/.`), stopping at the first
missing prefix. No `pathconf`, no per-directory byte-validity check.
`checkPOSIX` (`pathchk.go:133-156`): empty, `len(p) >= 256`
(NUL-inclusive), component > 14 bytes, portable charset
(`pathchk.go:172-185`, exactly `A-Za-z0-9._-`). `checkSpecial`
(`pathchk.go:158-170`): empty + leading-hyphen component. Diagnostics all
to `rc.Err`; exit 0/1; missing operand → UsageError exit 2
(`pathchk.go:36`).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Synopsis / `-p -P` parsing, `-pP` cluster, `--` terminator | verified | `cmds/pathchk/pathchk.go:27-31` | `cmds/pathchk/pathchk_test.go#TestPathchkRejectsLeadingHyphen` (`-P` + `./-bad`); probes `-pP ok` → 0, `-P -- -foo` → 1 | pflag handles clusters and `--` |
| Default: {PATH_MAX} length check | implementation_gap | `cmds/pathchk/pathchk.go:65-71,85-94` | `cmds/pathchk/pathchk_test.go#TestPathchkDefaultPathLimitIncludesTerminator` pins only the host-wide constant | The implementation uses a compile-time per-OS value instead of the underlying filesystem's limit and can misjudge paths on mounted filesystems with different limits |
| Default: {NAME_MAX} component check | implementation_gap | `cmds/pathchk/pathchk.go:66,72-77` | — | Hardcoded 255 rather than querying each component's containing directory; filesystems with a smaller or larger limit are misjudged |
| Default: unsearchable directory component | verified | `cmds/pathchk/pathchk.go:99-131` | `cmds/pathchk/pathchk_unix_test.go#TestPathchkRejectsUnsearchableDirectoryPrefix`, `#TestPathchkRejectsNonDirectoryPrefix`, `#TestPathchkRejectsDanglingSymlinkPrefix` | |
| Default: invalid byte sequence in containing directory | implementation_gap | no code (`checkDefault` has no charset or filesystem byte-validity check) | — | Issue 7 requires the containing-filesystem check. Assuming it is vacuous on selected hosts neither implements nor proves the required behavior |
| Default: missing components are not an error | verified | `cmds/pathchk/pathchk.go:110-112` | `cmds/pathchk/pathchk_test.go#TestPathchkAllowsMissingDirectoryPrefix` | Probe `missing/child` → 0 |
| `-p`: {_POSIX_PATH_MAX} 256 | verified | `cmds/pathchk/pathchk.go:141-144` | `cmds/pathchk/pathchk_test.go#TestPathchkPosixPathLimitIncludesTerminator` | Matches GNU's NUL-inclusive reading |
| `-p`: {_POSIX_NAME_MAX} 14 | evidence_gap | `cmds/pathchk/pathchk.go:145-149` | — | Probes: 15-char component → exit 1; 14-char → 0. Missing focused test |
| `-p`: portable filename character set | evidence_gap | `cmds/pathchk/pathchk.go:150-153,172-185` | only the passing case in `#TestPathchkPortable` | Probes: `aü`, `x:y`, `bad*name` all diagnosed, exit 1. Missing a focused rejection test |
| `-p` does NOT flag leading hyphen (RATIONALE) | evidence_gap | `cmds/pathchk/pathchk.go:172-185` (`-` in the portable set) | — | Probe `pathchk -p -- -foo` → 0 ✓; untested |
| `-P`: leading-hyphen component | verified | `cmds/pathchk/pathchk.go:163-167` | `cmds/pathchk/pathchk_test.go#TestPathchkRejectsLeadingHyphen` | |
| `-P`: empty pathname | verified | `cmds/pathchk/pathchk.go:159-162` | `cmds/pathchk/pathchk_test.go#TestPathchkEmptyPathnameOptions` | Also emits the default check's empty diagnostic (additive semantics — noisy but conformant) |
| `-P` additive to the default checks | verified | `cmds/pathchk/pathchk.go:47-52` | `cmds/pathchk/pathchk_test.go#TestPathchkSpecialAlsoChecksFilesystemPrefixes` | Only `-p` says "instead of"; matches GNU |
| Operands: one or more; per-operand aggregation | evidence_gap | `cmds/pathchk/pathchk.go:35-57` | — | Probe `pathchk -p ok 'bad*name'` → exit 1, one diagnostic ✓; multi-operand aggregation and missing-operand usage (exit 2) untested |
| Stdout not used / stderr only for diagnostics | verified | all Fprintf target `rc.Err` | `cmds/pathchk/pathchk_test.go` runPathchk helper fatals on any stdout | |
| Exit status 0 / >0 | verified | `cmds/pathchk/pathchk.go:38-57` | every test asserts codes | 0 all-pass, 1 any-fail, 2 usage (documented deviation) |

### Confirmed gaps (probe transcripts)

Three source-established implementation gaps have no runnable probe on this
host: default-mode PATH_MAX and NAME_MAX are compile-time constants
(`cmds/pathchk/pathchk.go:66,85-94`) rather than filesystem/containing-directory
queries, and byte-sequence validity is never checked. The ordinary-host probes
still pass — `""` → 1, `-P ""` → 1, `-P -- -foo` → 1, `-p -- -foo` → 0,
`-p` 15-char → 1 / 14-char → 0, `-p 'aü'` → 1, 256-byte component → 1 /
255 → 0, `plain/child` → "not a directory" exit 1, `missing/child` → 0 —
but they do not exercise a filesystem whose constraints differ from the
hardcoded assumptions.

---

## pax

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis (four forms):**

```text
pax [-dv] [-c|-n] [-H|-L] [-o options] [-f archive] [-s replstr]... [pattern...]
pax -r [-c|-n] [-dikuv] [-H|-L] [-f archive] [-o options]... [-p string]... [-s replstr]... [pattern...]
pax -w [-dituvX] [-H|-L] [-b blocksize] [[-a] [-f archive]] [-o options]... [-s replstr]... [-x format] [file...]
pax -r -w [-diklntuvX] [-H|-L] [-o options]... [-p string]... [-s replstr]... [file...] directory
```

**Options & option-arguments:** `-r` read/extract; `-w` write; `-a`
append to archive end; `-b blocksize` output block size (multiples of
512, ≤32256, `k`/`b`/`m` multipliers and `xN` products); `-c` match
complement; `-d` match the directory itself only, not the hierarchy
(all modes); `-f archive`; `-H`/`-L` command-line/full symlink following
(default: archive the symlink itself); `-i` interactive rename via
`/dev/tty` (blank line skips, `.` keeps the name, EOF stops); `-k` never
overwrite existing files; `-l` (copy mode) hard-link instead of copying
where possible; `-n` select only the FIRST archive member matching each
pattern; `-o keyword=value[,...]` — pax-format keywords
`delete=pattern`, `exthdr.name=`, `globexthdr.name=`,
`invalid={binary,bypass,rename,UTF-8,write}`, `linkdata`,
`listopt=format`, `times`, plus `charset`/`comment`/`hdrcharset`;
`-p string` with specifiers `a` (don't preserve atime), `e`
(everything), `m` (don't preserve mtime), `o` (preserve uid/gid), `p`
(preserve mode bits), last wins on conflict, a failure to preserve under
`-p` ⇒ diagnostic + nonzero exit; `-s /old/new/[gp]` ed-style
substitution — *old* is a **BRE**, *new* supports `&` and `\1..\9`, any
delimiter, first matching expression terminates, `p` prints `old >> new`
to stderr, an empty result skips the member, multiple `-s` allowed; `-t`
reset access times of files read; `-u` newer-only (read: extract only if
the member is newer than an existing file; write: supersede only if the
file is newer than the archived member; copy: replace only if newer);
`-v` — list mode: verbose `ls -l`-style table (with `==` linkname
notation for hard links); read/write/copy: pathnames to **stderr**;
`-x {cpio,pax,ustar}` output format (write mode; reading must handle all
supported formats); `-X` do not descend into different `st_dev`.

**Operands / arity / special tokens:** `pattern...` (fnmatch; default
select all; **any pattern matching no member ⇒ diagnostic on stderr +
non-zero exit**); `file...` (write/copy); trailing `directory` (copy
mode). Write mode with no `file` operands reads pathnames one-per-line
from stdin.

**Stdin:** the archive (list/read without `-f`); the pathname list
(write without operands); `-i` responses from `/dev/tty`.
**Environment:** locale vars (moot under the repo `LC_ALL=C` contract),
`TMPDIR`, `TZ`. **Stdout:** the list-mode member table, or the archive
when `-w` without `-f`. **Stderr:** `-v` progress pathnames, `-s…p`
rename reports, diagnostics. **Effects:** extraction creates
files/dirs/links, preserves mtime by default; failure to create a
file/link or find a file ⇒ diagnostic + nonzero exit **but processing
continues**; must not create a second copy when a link cannot be made.

**Diagnostics & exit status:** 0 all files processed successfully; >0
error.

### Bashy implementation (parser & behavior)

`cmds/pax/pax.go` registers only: `-r` `-w` `-f` `-v` `-x` (default
`"pax"`) `-p` `-s` `-i` `-l` `-k` `-u` `-d` `-a` `-c` `-n`
(`pax.go:68-82`). **`-b`, `-o`, `-t`, `-X`, `-H`, `-L` are not
registered at all** (unknown-flag exit 2), although the tool's own Usage
string (`pax.go:41-42`) advertises `-b`, `-t`, `-X`. Mode dispatch
`pax.go:100-109`. Extraction (`cmds/pax/modes.go:20-110`) plans via
`pkg/pax.PlanExtraction`, condemns the whole archive on any escaping
member (stricter than POSIX's per-member continue — a deliberate,
test-pinned security posture), overwrites by default, honors `-k`
(`modes.go:54-58,114-118`) and read-mode `-u` (`modes.go:120-124`).
Write (`modes.go:167-208`) supports tar (pax/ustar via Go `archive/tar`,
`modes.go:369-380`) and a hand-rolled cpio **newc writer**
(`modes.go:213-300`); `-a` opens `O_APPEND` (`modes.go:171-174`) with no
trailer handling; cpio `-a` loudly refused (`modes.go:283-285`). Copy
mode = write-into-pipe → extract (`modes.go:418-455`); `-l` never
consulted. List mode (`pax.go:132-157`) reads **tar only** via
`pkg/pax.ReadManifest`; `-v` prints mode/size/mtime but blank
owner/group and hardcoded nlink 1 (`pax.go:150-151`). `-s`
(`cmds/pax/select.go`): any delimiter, `&`/`\1..\9`,
first-match-stops, empty-drops — but the pattern is compiled with Go
`regexp.Compile` (`select.go:49`), i.e. Go/ERE-ish syntax, **not POSIX
BRE** (the repo's `pkg/bre` is not used), and the parsed `p` flag
(`select.go:62`) is never acted on. Pattern selection
(`select.go:121-142`) uses `path.Match` + directory-prefix expansion,
`-c` inverts; **no unmatched-pattern tracking** — and `o.interactive`,
`o.link`, `o.selectNoPattern` are set but never read anywhere. `-p` is a
single `strings.Contains(o.preserve, "m")` (`modes.go:150`) —
`a/e/o/p` and unknown characters silently accepted as no-ops. Write mode
with no operands writes an empty archive (never reads the stdin pathname
list).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Four-mode dispatch (list/`-r`/`-w`/`-r -w`) | verified | `cmds/pax/pax.go:100-109` | `cmds/pax/pax_test.go#TestWriteThenListThenExtractRoundTrips`, `#TestCopyModeUsesTheSameSafetyPathAsExtract` | content round-trips |
| `-f archive` | verified | `cmds/pax/pax.go:70,124-129`; `cmds/pax/modes.go:170-181` | `#TestWriteThenListThenExtractRoundTrips` | rc.Dir-relative resolution |
| Stdin archive (list/read, no `-f`) | verified | `cmds/pax/pax.go:126` | `#TestEscapingMemberRejectsArchiveWithoutWritingAnything` | |
| `-r` extract + overwrite-by-default | verified | `cmds/pax/modes.go:20-110` | `#TestOverwriteIsDefaultButEscapeStaysFatal` | |
| `-k` | verified | `cmds/pax/pax.go:77`; `cmds/pax/modes.go:54-58,114-118` | `#TestKeepDoesNotOverwrite` | |
| `-c` complement | verified | `cmds/pax/select.go:138-140` | `#TestPatternSelectionAndComplement` | |
| `-x ustar` write | verified | `cmds/pax/modes.go:370-380` | `#TestUstarFormatDropsUnrepresentableMetadata` | |
| `-x cpio` write | verified | `cmds/pax/modes.go:213-300` | `#TestCPIOFormatWritesNewcArchive` | newc, 512-padded |
| `-x pax` write (default) | verified | `cmds/pax/pax.go:72,186-191` | `#TestWriteThenListThenExtractRoundTrips` | Go FormatPAX |
| `-s` delimiter/first-match/empty-drop/`&`/backrefs | verified | `cmds/pax/select.go:21-117` | `#TestSubstitutionAcceptsAlternateDelimiters`, `#TestSubstitutionToEmptyDropsTheMember`, `#TestSubstitutionAmpersandAndGroups` | see the BRE gap below |
| Copy-mode directory operand check | verified | `cmds/pax/modes.go:424-429` | `#TestCopyModeUsesTheSameSafetyPathAsExtract` | not-a-directory ⇒ exit 1 |
| `-s` pattern is a POSIX BRE | implementation_gap | `cmds/pax/select.go:49` (`regexp.Compile`, Go syntax) | — | `\(...\)` are literal parens in Go regexp; `pkg/bre` exists in the repo but is unused here |
| `-s` trailing `p` flag (print rename to stderr) | implementation_gap | `cmds/pax/select.go:62` sets `s.print`; no consumer | — | Probe: `pax -f s.tar -s '/renamed/again/p'` → stderr empty, exit 0 (silent no-op) |
| Unmatched `pattern` ⇒ diagnostic + exit >0 | implementation_gap | `cmds/pax/select.go:121-142` (no match bookkeeping) | — | Probe: `pax -f a.tar 'nope*'; echo $?` → no output, no diagnostic, **exit 0** |
| `-b blocksize` | implementation_gap | not registered (`cmds/pax/pax.go:66-83`); advertised in Usage `pax.go:41` | — | Probe: `pax -w -b 10k -f b.tar dir` → `unknown shorthand flag: 'b'`, exit 2 |
| `-o options` (delete=, exthdr.name=, globexthdr.name=, invalid=, linkdata, listopt=, times, …) | implementation_gap | not registered | — | Probe: `pax -w -o invalid=UTF-8 …` → exit 2. The entire keyword surface is absent |
| `-t` (reset atime) | implementation_gap | not registered; advertised in Usage `pax.go:41` | — | Probe: exit 2 |
| `-X` (same device) | implementation_gap | not registered; advertised in Usage `pax.go:41-42` | — | Probe: exit 2 |
| `-H` / `-L` symlink following | implementation_gap | not registered; the walker always Lstats (`cmds/pax/modes.go:304,350`) | — | Probes: exit 2; only the POSIX default (archive the symlink) is available |
| `-i` interactive rename via /dev/tty | implementation_gap | `cmds/pax/pax.go:75` registers; `o.interactive` never read | — | Probe: `printf '.\n…' \| pax -r -i -f a.tar` extracted everything with no prompting, exit 0 — silent no-op |
| `-l` copy-mode hard links | implementation_gap | `cmds/pax/pax.go:76` registers; `o.link` never read (`modes.go:418-455`) | — | Probe: `pax -r -w -l dir dstl` → exit 0 but different inodes — silent copy instead of link |
| `-n` first-match-only | implementation_gap | `cmds/pax/pax.go:82` registers; `selectNoPattern` never read | — | Probe: `pax -f s.tar -n 'renamed/*'` listed 2 members, not 1 |
| `-p a/e/o/p` + invalid-specifier rejection | implementation_gap | `cmds/pax/pax.go:73`; only `m` consulted (`cmds/pax/modes.go:150`) | — | Probes: `-p o` → exit 0 with uid/gid never restored; `-p zz` → exit 0 silently. `-p m` incidentally disables atime restoration too |
| `-a` append (tar) | implementation_gap | `cmds/pax/modes.go:171-174` (raw O_APPEND, no trailer rewind) | — | Probe: `pax -w -a -f a.tar extra.txt` → **exit 0**, but a subsequent list does NOT show `extra.txt` — appended data lands after the old end-of-archive blocks and is invisible; silent data loss (cpio append is at least loudly refused, `modes.go:283`) |
| `-u` read mode | evidence_gap | `cmds/pax/modes.go:120-124` | — | Probe confirmed `-r -u` kept a newer local file; no repo test |
| `-u` write/copy write-side | implementation_gap | writeMode never compares against existing archive members | — | Flag accepted; supersede-if-newer semantics absent (silent no-op in `-w`) |
| `-d` write mode | evidence_gap | `cmds/pax/modes.go:333,399` | — | Probe: `-w -d` archived only `dir/`; no repo test |
| `-d` list/read/copy modes | implementation_gap | `cmds/pax/select.go:133` always expands a directory pattern to its subtree regardless of `-d` | — | POSIX: `-d` must stop pattern matches at the directory itself in all modes |
| `-v` list mode (`ls -l` table, `==` link notation) | implementation_gap | `cmds/pax/pax.go:150-151` | — | Probe: blank owner/group, hardcoded nlink 1, no `==` notation — not an `ls -l` listing |
| `-v` read/write (pathnames to stderr) | evidence_gap | `cmds/pax/modes.go:105-107,328-330,394-396` | — | Probe: `-w -v` wrote 4 pathnames to stderr; no repo test |
| cpio format reading/listing | implementation_gap | `cmds/pax/pax.go:139` / `cmds/pax/modes.go:35-44` (tar-only readers) | — | Probe: `pax -f x.cpio` → `pax: archive/tar: invalid tar header`, exit 1. POSIX pax must read the formats it supports; cpio is write-only here |
| Write mode: pathname list from stdin when no operands | implementation_gap | `cmds/pax/modes.go:193-199` loops over `files` only; rc.In never read in `-w` | — | Probe: `printf 'dir/a.txt\n' \| pax -w -f stdin.tar` → exit 0, archive **empty** (silent) |
| Continue-after-member-error + exit >0 | evidence_gap | `cmds/pax/modes.go:100-104` (status=1, continue) | — | conformant shape; no focused test |
| Escaping-member handling | implementation_gap | `cmds/pax/modes.go:47-67` | `#TestEscapingMemberRejectsArchiveWithoutWritingAnything` pins the deviation | Any escaping member condemns the whole archive before safe members are extracted. Issue 7 requires a diagnostic, nonzero status, and continued processing after a member cannot be created; the deliberate security posture is still a conformance gap |
| Exit statuses 0/>0 (usage 2, repo deviation) | verified | `cmds/pax/pax.go:83-98` | `#TestWriteThenListThenExtractRoundTrips` et al. | unknown flags exit 2 loudly |

### Confirmed gaps (probe transcripts)

- `pax -f a.tar 'nope*'; echo $?` → no output, **exit 0**. Required:
  stderr diagnostic + exit >0 for an unmatched pattern.
- `pax -w -b 10k -f b.tar dir` → `pax: unknown shorthand flag: 'b'`,
  exit 2 (the Usage string itself advertises `-b`). Likewise `-o`, `-t`,
  `-X`, `-H`, `-L` → exit 2 each.
- `pax -w -a -f a.tar extra.txt` → exit 0, then `pax -f a.tar` lists
  only the original members — the appended member is silently lost (tar
  trailer not rewound).
- `pax -w -x cpio -f x.cpio dir` (exit 0) then `pax -f x.cpio` →
  `pax: archive/tar: invalid tar header`, exit 1 — cpio is write-only.
- `pax -r -w -l dir dstl` → exit 0, different inodes — `-l` is a silent
  no-op. `pax -f s.tar -n 'renamed/*'` → 2 members — `-n` is a silent
  no-op. `printf '.\n...' | pax -r -i -f a.tar` → no rename prompt —
  `-i` is a silent no-op.
- `pax -r -p zz -f a.tar` → exit 0, silent; `pax -r -p o …` → exit 0
  with no ownership restoration.
- `pax -f s.tar -s '/renamed/again/p'` → stderr empty; the `p`
  substitution flag never reports renames.
- `printf 'dir/a.txt\n' | pax -w -f stdin.tar` → exit 0, empty archive;
  the stdin pathname list is never read.
- `pax -v -f a.tar` → `-rw-r--r--  1                          6 Aug 24
  18:53 dir/a.txt` — owner/group blank, nlink hardcoded.
- An archive containing one escaping and one safe member is rejected in
  full at `cmds/pax/modes.go:47-67`; `#TestEscapingMemberRejectsArchiveWithoutWritingAnything`
  pins that whole-archive abort instead of Issue 7's diagnose, fail, and
  continue-after-member-error behavior.

---

## pr

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pr.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `pr [+page] [-column] [-adFmrt] [-e[char][gap]]
[-h header] [-i[char][gap]] [-l lines] [-n[char][width]] [-o offset]
[-s[char]] [-w width] [-fp] [file...]` (`-l` and `-f` XSI-shaded in
part; see below). Syntax-guideline exemptions (normative): `+page` uses
a `+` delimiter; *page*/*column* may be multi-digit; **the `-s`, `-e`,
`-i`, `-n` option-arguments must be attached** (a detached following
token is a *file* operand); `-h`/`-l`/`-o`/`-w` accept attached or
detached arguments.

**Options & option-arguments:** `+page` begin output at page *page*.
`-column` multi-column (default 1), text written **down** each column;
should not be used with `-m`; **`-e` and `-i` shall be assumed for
multiple text-column output**; with `-t`, use the minimum number of
lines (balancing). `-a` — with `-column`, fill columns **across**
(round-robin). `-d` double-space. `-e[char][gap]` input tab expansion:
positions gap+1, 2·gap+1, …; **gap zero or omitted defaults to 8**;
non-digit first char = input tab char. `-f` [XSI] form-feed page
separators **and pause before the first page if stdout is a terminal**.
`-F` form-feed separators (no pause). `-h header` replace the *file*
field of the page header. `-i[char][gap]` in **output**, replace spaces
reaching positions gap+1, 2·gap+1, … with tabs; gap 0/omitted → every 8.
`-l lines` page length (default 66); **if lines ≤ header+trailer depth
(10), suppress header and trailer as if `-t`**. `-m` merge: one line
from each file side by side in equal fixed-width columns; ≥9 file
operands supported. `-n[char][width]` line numbering, width default
**5**, in the first *width* positions of each text column (default
output) or of each `-m` line; *char* appended after the number, default
tab. `-o offset` prefix each line with *offset* spaces, in addition to
`-w`. `-p` pause before each page if stdout is a terminal: alert to
**stderr**, wait for carriage return on **/dev/tty**. `-r` write no
diagnostics on failure to open files (exit status unaffected).
`-s[char]` separate text columns by the single *char* (default tab)
**instead of** the padding spaces. `-t` omit the 5-line header and
5-line trailer, quit after the last line with no page-fill. `-w width`
line width for **multi-column output only**; default **72 without `-s`,
512 with `-s`**; single-column lines are never truncated; multi-column
lines that do not fit a text column shall be truncated.

**Operands / arity / special tokens:** `file...`; no operands or `-` →
stdin. APPLICATION USAGE: a leading-`+` first operand must be
protectable with `--`.

**Stdin:** when no operands or `-`; `/dev/tty` supplies `-p` responses.
**Environment:** locale vars, **`LC_TIME`** (header date format),
**`TZ`** (header timezone). **Stdout:** paginated output; default
66-line pages = 5-line header (2 blank, header line, 2 blank) + body +
5-line blank trailer; the header line is `"\n\n%s %s Page %d\n\n\n"`
where in the POSIX locale the date field is `date "+%b %e %H:%M %Y"`
evaluated at the **file's mtime** (current time for stdin); the *file*
field is a null string for stdin, or the `-h` argument. **Stderr:**
diagnostics and the `-p` alert; **if stdout is a terminal, diagnostics
are deferred until pr completes**. **Effects:** interrupt while writing
to a terminal flushes accumulated error messages.

**Diagnostics & exit status:** 0 successful completion; >0 an error
occurred. Consequences of errors: default.

### Bashy implementation (parser & behavior)

`cmds/pr/pr.go` (1052 lines; package doc `pr.go:1-12` self-describes as
a "GNU pr subset"). Parser: `scanColumnOption` (`pr.go:280-334`)
pre-rewrites **pure-digit** `-N` args to `--columns=N` (`pr.go:327-329`)
and rescues attached optional args for s/e/i/n when clustered after only
`adFfJmrtT` (`pr.go:296-319`); everything else goes to pflag.
`-e/-n/-s/-i` use `NoOptDefVal` (`pr.go:81,86,93,102`) so a detached
following token is a file operand — the POSIX attachment rule holds.
`-h/-l/-o/-w` are normal pflag options (attached or detached).
`+FIRST[:LAST]` is recognized as an operand **after** flag parsing
(`pr.go:195-205`) — so `--` does not protect a leading-`+` file. GNU
extensions carried: `--pages`, `-D` date-format, `-W`, `-N`, `-J`
(silent no-op, `pr.go:100,170`), `-T`, `-S`. `-i` is parsed but
**discarded** (`_ = outputTabs`, `pr.go:171`). `-f` is a plain alias of
`-F` (`pr.go:97,161` — no tty pause). No `-p` flag exists. Defaults:
length 66 (`pr.go:71`), width 72 (`pr.go:72`) with **no 512-with-`-s`
logic**; ≤10 implies `-t` (`pr.go:180`); body = length−10 (`pr.go:184`),
halved for `-d` (`pr.go:186-191`). `-e` expansion happens only
`if fs.Changed("expand-tabs")` (`pr.go:160`) — not assumed for
multi-column. Header: GNU-style centered line with ISO date
`"2006-01-02 15:04"` (`headerLine` `pr.go:878-898`), file mtime for
files / now for stdin (`open` `pr.go:336-353`), stdin label = null
string. Multi-column (`printColumnChunk` `pr.go:792-873`): down-fill
with minimal rows, truncation only when `separator == ""`
(`pr.go:814,840`). Merge (`printMerge` `pr.go:625-692`, `mergeLine`
`pr.go:715-757`): always truncates cells to width/columns−1
(`pr.go:731-740`), pads to fixed column width **and then** appends the
`-s` char (`pr.go:744-753`); the merge header stamp is `time.Now()`
(`pr.go:641-644`). Merge open failure aborts the whole merge with exit
1, and its three diagnostics use `\\n` — a literal backslash-n, no
newline (`pr.go:248,266,270`). Exit: 0 ok, 1 file/write errors, 2 usage.
No tty detection anywhere (no deferred diagnostics, no `-p`/`-f`
pause).

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| `+page` begins output at page N | verified | `cmds/pr/pr.go:195-205,972-994` | `cmds/pr/pr_test.go#TestPRPlusOperandPageRange` | `+0` → usage exit 2; `+FIRST:LAST` is a GNU-superset extension |
| `--` protects a leading-`+` file operand | implementation_gap | `cmds/pr/pr.go:195-205` (the `+`-scan runs on post-`--` operands) | — | Probe: with a real file named `+2`, `pr -t -- +2` → empty output, exit 0 (consumed as a page range). POSIX APPLICATION USAGE requires `--` to make it a file |
| `-column` down-fill; minimum lines with `-t` | verified | `cmds/pr/pr.go:799,820-856` | `#TestPRVerticalColumns`, `#TestPRVerticalColumnsUnevenFinalPage` | Probe `-t -w10 -2` on 5 lines → 3 balanced rows ✓ |
| `-column` clustered with other flags (`pr -3d`, the spec's own example) | implementation_gap | `cmds/pr/pr.go:327-329` (only pure-digit `-N` rewritten) | — | Probe: `pr -3d -t -w 20` → `unknown shorthand flag: '3' in -3d`, exit 2. Loud, but POSIX-exercised syntax rejected |
| `-e`/`-i` assumed for multi-column output | implementation_gap | `cmds/pr/pr.go:160` (`expandTabs: fs.Changed(...)`), `pr.go:171` (`-i` discarded) | — | Probe: `printf 'a\tb\nc\td\n' \| pr -t -2 -w 20` → raw tab retained in cells. POSIX requires expansion to be assumed |
| `-e[char][gap]` attached parsing; defaults tab/8 | verified | `cmds/pr/pr.go:85-86,148-151,996-1040` | `#TestPRExpandTabs`, `#TestPROptionalExpandArgument` (`-e4`,`-eX4`,`-eX`) | |
| `-e 4` detached → `4` is a file operand | evidence_gap | `cmds/pr/pr.go:86` (NoOptDefVal) | only the `-s` analogue `#TestPRBareSeparatorDoesNotConsumeFile` | Probe: `pr -t -e 4` → `pr: 4: No such file or directory`, exit 1 ✓; untested (same for `-n 5`) |
| `-e0` (gap zero → default 8) | implementation_gap | `cmds/pr/pr.go:1015-1018` (`n <= 0` → error) | — | Probe: `pr -t -e0` → `invalid expand-tabs value: "0"`, usage error. POSIX: gap 0 shall default to 8. Also applies to `-eX0` and `-i…0` |
| `-f` (XSI: form feed + first-page pause on a terminal) | implementation_gap | `cmds/pr/pr.go:97,161` (pure alias of `-F`) | `#TestPRFormFeedLowerEqualsUpper` *pins the deviation* | No tty detection, no /dev/tty read, no stderr alert |
| `-F` form-feed page separators | verified | `cmds/pr/pr.go:96,161,522-526,864-867` | `#TestPRFormFeedTrailer` | |
| `-h header` replaces the file field | verified | `cmds/pr/pr.go:75,156,371-374,489` | `#TestPRCustomHeaderAndTOmitPagination` | attached `-hTTL` also works |
| Header line content: `"%s %s Page %d"`, POSIX-locale date `%b %e %H:%M %Y` | implementation_gap | `cmds/pr/pr.go:878-898` (ISO `2006-01-02 15:04`, GNU centered layout) | `#TestPRDefaultPageStructure` *pins the GNU format* | Probe line 3: `2020-01-02 03:04 … f1 … Page 1`. POSIX requires `Jan  2 03:04 2020 f1 Page 1`. Prime PCTS failure candidate |
| Header date = file mtime | verified | `cmds/pr/pr.go:348-352` | `#TestPRDefaultPageStructure` (Chtimes-pinned) | |
| Stdin: current time + null file field in header | evidence_gap | `cmds/pr/pr.go:337-341` | — | Probe: stdin header shows date + `Page 1`, no label ✓; untested |
| 66-line page = 5-line header + body + 5-line trailer, last page padded | verified | `cmds/pr/pr.go:34-37,180-185,502-533` | `#TestPRDefaultPageStructure` (exact 66-line golden) | Probe: seq 100 → 132 lines ✓ |
| `-i[char][gap]` output space→tab conversion | implementation_gap | `cmds/pr/pr.go:101-102,171` (parsed, discarded) | `#TestPRNewFlagAliases`, `#TestPRClusteredOptionalOutputTabs` *pin the no-op* | Probe: spaces unchanged. **Silent acceptance** — violates POSIX and the repo's never-silent-fallthrough rule |
| `-l lines` default 66; ≤10 implies `-t` | verified | `cmds/pr/pr.go:71,178-185` | `#TestPRShortPageLengthImpliesOmitHeader`, `#TestPRRejectsBadLength` | Probe boundary: `-l 10` → 1 line, `-l 11` → 11 lines ✓ |
| `-m` merge, equal fixed columns, ≥9 files | verified (core) | `cmds/pr/pr.go:241-274,625-757` | `#TestPRMerge`, `#TestPRMergeThreeFilesAndPagination` | arbitrary file count; `-m -column` conflict → exit 1 ✓ |
| `-m` with `-s`: single char **instead of** space padding | implementation_gap | `cmds/pr/pr.go:744-753` (pads to fixed width, then appends sep) + truncation `pr.go:731-740` regardless of `-s` | `#TestPRMerge` *pins* `"a\t :1"` | Probe: `pr -m -t -s: f1 f2` → `1^I^I^I^I   :101`; POSIX: `1:101` |
| `-m` header date field (file mtime vs now) | evidence_gap | `cmds/pr/pr.go:641` (`time.Now()`) | — | Spec ambiguous for multiple files; flagged, not counted as a proven deviation |
| `-n[char][width]` attached parsing, defaults tab/5; per-column and per-`-m`-line placement | verified | `cmds/pr/pr.go:78-81,144-147,900-916,812,667-669` | `#TestPROptionalNumberArgument`, `#TestPRNumberIndentAndDoubleSpace`, `#TestPRMergeLineNumbering` | |
| `-o offset` | verified | `cmds/pr/pr.go:82,134-136,909-911,824` | `#TestPRNumberIndentAndDoubleSpace`, `#TestPRRejectsBadIndent` | |
| `-p` pause per page (alert to stderr, read /dev/tty) | implementation_gap | no flag registered (`cmds/pr/pr.go:68-102`) | — | Probe: `pr -p` → `unknown shorthand flag: 'p'`, exit 2. Loud refusal, but a required POSIX option is absent |
| `-r` suppress open diagnostics; exit still >0 | verified | `cmds/pr/pr.go:83,217-222` | `#TestPRNoFileWarnings` | Probes: `-r missing` → silent exit 1; without `-r` → diagnostic + exit 1 ✓ |
| `-s[char]` attached-only; bare `-s` = tab, doesn't consume an operand | verified (parsing) | `cmds/pr/pr.go:90-93` (NoOptDefVal `"\t"`) | `#TestPRBareSeparatorDoesNotConsumeFile` | Bare `-s` default-tab *output* in non-merge multi-column untested |
| `-s` + explicit `-w`: truncation to column width | implementation_gap | `cmds/pr/pr.go:814,840,848` (truncate/pad only when `separator == ""`) | `#TestPRCustomSeparatorDoesNotTruncateColumns` *pins* no-truncation | Probe: `pr -t -s, -2 -w 20` on 30-char lines → one 61-char untruncated line. POSIX: lines that do not fit shall be truncated |
| `-w` default 512 when `-s` given | implementation_gap | `cmds/pr/pr.go:72` (always 72), `pr.go:167-169` (no `-s` coupling) | — | Probe: `pr -s: -2` header line is 72 chars wide, not 512 |
| `-w` multi-column truncation w/o `-s`; single-column never truncated | verified | `cmds/pr/pr.go:814,900-916` | `#TestPRSingleColumnNeverTruncatedByDefault`, `#TestPRColumnsReserveBlankBetweenFullWidthCells` | only GNU `-W` truncates single-column |
| `-a` round-robin across | verified | `cmds/pr/pr.go:829-838` | `#TestPRAcrossColumns` | `-a -m` conflict → exit 1 with diagnostic |
| `-d` double space | verified | `cmds/pr/pr.go:186-191,514-517,858-861` | `#TestPRNumberIndentAndDoubleSpace`, `#TestPRVerticalColumnsInteractions` | |
| `-t` omit header/trailer, stop after last line | verified | `cmds/pr/pr.go:73,180-185` | `#TestPROmitHeaderPassesContent`, `#TestPRInputFormFeedsBreakPages` | |
| Operands: none / `-` → stdin | verified | `cmds/pr/pr.go:206-208,336-342` | stdin path exercised throughout `cmds/pr/pr_test.go` | explicit `-` untested but same code path |
| `-m` open-failure diagnostics well-formed | implementation_gap | `cmds/pr/pr.go:248,266,270` — format strings contain `\\n` (escaped), not `\n` | — | Probe: `pr -m -t f1 nosuch` → stderr ends `…No such file or directory\n` with a *literal* backslash-n and no terminating newline; the merge also aborts entirely (no output for the readable file) |
| Diagnostics deferred until completion when stdout is a terminal | implementation_gap | no tty detection anywhere in `cmds/pr/pr.go` | — | Normative DESCRIPTION requirement; unreachable from the RunContext harness today |
| Environment: TZ / LC_TIME | evidence_gap | `cmds/pr/pr.go:883` (`stamp.Format` in time.Local honors TZ); LC_TIME unimplemented (fixed layout) | — | The repo LC_ALL=C contract makes LC_TIME moot, but even the POSIX-locale date format is wrong (header row above) |
| Exit status 0 / >0; diagnostics on stderr | verified | `cmds/pr/pr.go:214-236` | `#TestPRNoFileWarnings`, `#TestPRRejectsBadLength` | usage 2 (documented repo deviation) |

### Confirmed gaps (probe transcripts)

1. **Header line format** — `pr f1` (mtime 2020-01-02 03:04) → line 3
   `2020-01-02 03:04 … f1 … Page 1` (GNU centered, ISO date). Required
   (POSIX locale): `Jan  2 03:04 2020 f1 Page 1`. Likely the largest
   single source of the +15 VSC-PCTS pr failures — every paginated test
   sees the header.
2. **`-p` missing** — `printf 'a\n' | pr -p` → `unknown shorthand flag:
   'p'`, exit 2.
3. **`-i` silent no-op** — `printf 'a        b\n' | pr -t -i` → spaces
   unchanged, exit 0.
4. **`-e`/`-i` not assumed for multi-column** — `printf 'a\tb\nc\td\n' |
   pr -t -2 -w 20` → raw tab inside cells.
5. **`-e0` rejected** — `pr -t -e0` → `invalid expand-tabs value: "0"`,
   exit 2. Required: gap 0 defaults to 8.
6. **`-s` layout in `-m`** — `pr -m -t -s: f1 f2` → `1^I^I^I^I   :101`.
   Required: `1:101`.
7. **`-s` disables truncation even with explicit `-w`; `-w` default not
   512 under `-s`** — `pr -t -s, -2 -w 20` on 30-char lines → 61-char
   untruncated line; `pr -s: -2` header width stays 72.
8. **Clustered `-column` rejected** — `pr -3d …` (the spec's own
   EXAMPLE) → `unknown shorthand flag: '3'`, exit 2.
9. **`--` does not protect a `+` file** — with a real file `+2`,
   `pr -t -- +2` prints nothing, exit 0.
10. **Merge diagnostics malformed** — `pr -m -t f1 nosuch` → stderr with
    a literal `\n` and no newline; the whole merge aborts.
11. **`-f` never pauses** — behaves identically to `-F` (test-pinned);
    the XSI first-page tty pause is absent.

---

## ps

Spec: <https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html>

### Normative interface (Issue 7, 2016 ed.)

**Synopsis:** `ps [-aA] [-defl] [-g grouplist] [-G grouplist]
[-n namelist] [-o format]... [-p proclist] [-t termlist] [-u userlist]
[-U userlist]` (`-d -e -f -g -l -n -u` XSI-shaded).

**Options & option-arguments:** `-a` all processes associated with
terminals **excluding session leaders**; `-A` all; `-d` all except
session leaders [XSI]; `-e` ≡ `-A` [XSI]; `-f` full listing, columns
`UID PID PPID C STIME TTY TIME CMD` [XSI]; `-l` long listing, columns
`F S UID PID PPID C PRI NI ADDR SZ WCHAN TTY TIME CMD` [XSI];
`-g grouplist` processes whose **session leaders** are in the list
[XSI]; `-G grouplist` real group IDs; `-n namelist` alternative system
namelist [XSI]; `-o format` (repeatable; comma/blank-separated variable
names; `name=header` renames, blank header allowed — the header line is
suppressed only if all headers are null; a null-header field keeps its
default width); variables: `ruser` (RUSER, real uid), `user` (USER,
effective), `rgroup` (RGROUP), `group` (GROUP), `pid` (PID), `ppid`
(PPID), `pgid` (PGID), `pcpu` (%CPU), `vsz` (VSZ, KiB), `nice` (NI),
`etime` (ELAPSED, `[[dd-]hh:]mm:ss`), `time` (TIME, `[dd-]hh:mm:ss`),
`tty` (**TT**), `comm` (COMMAND), `args` (COMMAND); only comm/args may
contain blanks; `-p proclist`, `-t termlist`, `-u userlist` (effective
uid or login name) [XSI], `-U userlist` (real uid or login name).

**Operands:** none. **Stdin:** not used. **Environment:** `COLUMNS`
(line-width override), `LC_TIME`, `TZ`, plus locale vars. **Stdout:**
column headings + one line per process, **fields blank-separated**.
**Stderr:** diagnostics. **Effects:** read-only report; **default
selection = processes with the invoker's effective UID AND the invoker's
controlling terminal**; any selection option replaces the default with
the inclusive OR of all given selection criteria; default columns should
include PID, TTY, TIME, CMD.

**Diagnostics & exit status:** 0 success; >0 error.

### Bashy implementation (parser & behavior)

`cmds/ps/ps.go`: flags at `ps.go:51-64` (`-A -a -d -e -f -l -g -G -n -o
-p -t -u -U`); operands rejected exit 2 (`ps.go:69-71`); `-n` loudly
rejected exit 2 (`ps.go:72-74`). Enumeration via
`github.com/tklauser/ps` (`ps.go:99`). Platform enrichment is
build-tagged: `cmds/ps/process_linux.go` fills pgid/sid/tty/cpu/nice/vsz
from `/proc/<pid>/stat`; **`cmds/ps/process_other.go` (`//go:build
!linux`, what darwin compiles) is a total no-op: `enrich(*process) {}`
and `currentUID() = -1`** — it does NOT return an "unsupported" error;
it silently ships zero/blank tty, pgid, sid, nice, vsz, and cpu, and the
−1 UID makes the default selection accept **every** process
(`ps.go:137-139`). Selection (`ps.go:121-140`): explicit lists are OR-ed
(the union rule); `-a` = tty non-blank (no session-leader exclusion);
`-d` = `pid != sid`; `-u`/`-U` both match the same single `p.uid`; `-g`
matches the process-group id, not session leaders. Format
(`ps.go:142-183`): defaults `pid,tty,time,comm`; `-f` →
`user,pid,ppid,pcpu,start,tty,time,args`; `-l` →
`f,s,uid,pid,ppid,pcpu,pri,nice,addr,sz,wchan,tty,time,comm`; all POSIX
`-o` names accepted plus extensions; `name=header` and null-header
suppression implemented; default headers deviate: `tty`→`TTY` (POSIX
`TT`), `args`→`CMD` (POSIX `COMMAND`). Values (`ps.go:219-266`):
`f/pri/addr/wchan` hardcoded `-`, `s` hardcoded `?`, `pcpu` hardcoded
`0.0`; `etime` always `HH:MM:SS` (`ps.go:327-333`), `time` always
`HH:MM:SS` (`ps.go:334-337`) — neither matches the POSIX shapes. Output
via `text/tabwriter` with `AlignRight` (`ps.go:186`) — **adjacent
columns are not blank-separated** (e.g. `TIMECOMMAND`,
`00:00:00kernel_task`). `COLUMNS`/`LC_TIME` are never consulted.

### Classification

| Element | Class | Source evidence | Test evidence | Detail |
|---|---|---|---|---|
| Synopsis parsing, no operands, usage exit 2 | verified | `cmds/ps/ps.go:50-74` | `cmds/ps/ps_test.go#TestPSRejectsUnknownFormat` (exit-2 path) | Probe: `ps -t "not a tty"` extra operand → exit 2 |
| `-p proclist` (comma/blank lists, numeric) | verified | `cmds/ps/ps.go:61,78,268-287` | `cmds/ps/ps_test.go#TestPSOwnPIDAndFormat` | Probe `ps -p $$` → one row |
| `-o` unknown-name rejection | verified | `cmds/ps/ps.go:158-159` | `cmds/ps/ps_test.go#TestPSRejectsUnknownFormat` | exit 2 |
| `-o name=` null-header rules | verified | `cmds/ps/ps.go:163,185-201` | `cmds/ps/ps_test.go#TestPSExplicitEmptyHeadings` | Probe `ps -o pid=MYPID` renames |
| `-A`/`-e` all-processes selection | evidence_gap | `cmds/ps/ps.go:51,54,76,127-129` | — | Probe `ps -e` → 696 processes incl. root; no repo test of selection |
| Union-of-selection-criteria rule | evidence_gap | `cmds/ps/ps.go:122-126` | — | implemented; untested |
| Multiple `-o` / comma+blank separation | evidence_gap | `cmds/ps/ps.go:153-154` | — | implemented; untested |
| `-G` real-group selection | evidence_gap | `cmds/ps/ps.go:58,84,123` | — | whether `p.gid` is real vs effective gid is not established; no test |
| Default selection (same EUID **and** same controlling terminal) | implementation_gap | `cmds/ps/ps.go:137-139` + `cmds/ps/process_other.go:6` (`currentUID() = -1`) | — | Probe (darwin): bare `ps` printed **all 600+ processes incl. root daemons**. Even on linux, the controlling-terminal half of the default is absent (EUID-only) |
| darwin/!linux platform leg | implementation_gap | `cmds/ps/process_other.go:1-6` (enrich no-op, uid −1) | — | Does NOT error; silently degrades: tty `?`, TIME `00:00:00`, %CPU `0.0`, VSZ/SZ 0, NI 0, PGID 0. Violates the repo's own "clear error, never silent approximation" policy — partial data, not a refusal |
| `-a` (terminals, minus session leaders) | implementation_gap | `cmds/ps/ps.go:133-135` (tty≠"" only; no session-leader exclusion) | — | Probe (darwin): header only, 0 rows; leaders not excluded on linux either |
| `-d` (all except session leaders) | implementation_gap | `cmds/ps/ps.go:130-132` (`pid != sid`; sid always 0 off-linux) | — | Probe: 694 rows — effectively "all", leaders included |
| `-t termlist` | implementation_gap | `cmds/ps/ps.go:62,93,123,300`; `process_other.go` no-op tty | — | Probe `ps -t ttys000` → 0 rows on darwin |
| `-g` (POSIX: session-leader list) | implementation_gap | `cmds/ps/ps.go:57,123` matches `p.pgid` (process group), pgid 0 on darwin | — | wrong axis and dead on darwin |
| `-u` (effective) vs `-U` (real) distinction | implementation_gap | `cmds/ps/ps.go:123` — both match the one `p.uid`; likewise `ruser`/`user`, `rgroup`/`group` (`ps.go:229-232`) | — | real and effective conflated by construction; probe: `ps -o ruser,user,rgroup,group -p 1` → `root root wheelwheel` |
| `-f` column set (`UID PID PPID C STIME TTY TIME CMD`) | implementation_gap | `cmds/ps/ps.go:147` | — | Probe header: `USER PID PPID %CPU STIME TTY TIMECMD` — `UID`→`USER`, `C`→hardcoded `%CPU`, TIME glued to CMD |
| `-l` column set (`F S UID PID PPID C PRI NI ADDR SZ WCHAN TTY TIME CMD`) | implementation_gap | `cmds/ps/ps.go:145,221-224` | — | `%CPU` where POSIX has `C`, `COMMAND` where `-l` defines `CMD`; F/PRI/ADDR/WCHAN literal `-`, S literal `?`, SZ 0 — placeholders, not data |
| `-n namelist` | implementation_gap (loud) | `cmds/ps/ps.go:59,72-74` | — | Probe: exit 2, `option -n is not supported on this platform-independent implementation` — repo-policy-conformant refusal; the XSI option is unimplemented |
| `-o tty` default header `TT` | implementation_gap | `cmds/ps/ps.go:181` (`"tty":"TTY"`) | — | Probe: header `TTY` |
| `-o args` default header `COMMAND` | implementation_gap | `cmds/ps/ps.go:181` (`"args":"CMD"`) | — | Probe: header `CMD` |
| `etime` format `[[dd-]hh:]mm:ss` | implementation_gap | `cmds/ps/ps.go:327-333` (always `%02d:%02d:%02d`, unbounded hours) | — | Probe: PID 1 → `761:28:24`; POSIX shape would be `31-17:28:24` |
| `time` format `[dd-]hh:mm:ss` | implementation_gap | `cmds/ps/ps.go:334-337` + cpu never populated off-linux | — | always `00:00:00` on darwin |
| `pcpu` value | implementation_gap | `cmds/ps/ps.go:239-240` (hardcoded `"0.0"`) | — | never computed on any platform |
| Blank-separated output fields | implementation_gap | `cmds/ps/ps.go:186` (tabwriter AlignRight, padding 1) | — | Probes: `TIMECOMMAND`, `00:00:00kernel_task`, `wheelwheel` — adjacent columns glued with zero blanks |
| COLUMNS / LC_TIME environment | implementation_gap | no reference anywhere in `cmds/ps` | — | COLUMNS never consulted (no width limiting); LC_TIME moot under LC_ALL=C; TZ honored incidentally (`ps.go:251`) |
| Exit status 0 / >0 | verified | `cmds/ps/ps.go:118` (0), `ps.go:100-102` (1), usage 2 | `cmds/ps/ps_test.go#TestPSOwnPIDAndFormat` (0), `#TestPSRejectsUnknownFormat` (2) | |

### Confirmed gaps (probe transcripts)

(darwin host; `./bin/coreutils ps …`)

- `ps; echo $?` → exit 0 but lists every process on the system (600+
  rows), all with `TTY=?`, `TIME=00:00:00`. Required: only processes
  with the invoker's EUID and controlling terminal.
- `ps -a` → header only, zero rows. `ps -d | wc -l` → 694. `ps -t
  ttys000` → zero rows. `ps -g 1` → zero rows (wrong axis: pgid).
- `ps -f | head -3` → `USER PID PPID %CPU STIME TTY TIMECMD`;
  `ps -l | head -3` → placeholder F/S/PRI/ADDR/WCHAN columns.
- `ps -o etime,time,tty,args -p 1` → `761:28:24 00:00:00 ?launchd` —
  etime shape, zero TIME, `TTY`/`CMD` headers, glued columns.
- `ps -o ruser,user,rgroup,group -p 1` → `root root wheelwheel`.
- `ps -n foo` → exit 2 with a clear unsupported message (loud refusal).

---

## Confirmed implementation gaps ranked by likely VSC-PCTS TP impact

Ranking inputs: the 2026-08-01 measurement in
`docs/posix-vsc-pcts-status.md` (deltas vs the GNU arm: **pr +15, od +5,
mkfifo +5, mkdir +1**; **ps fails identically in both arms** — zero
delta; mv/paste/pathchk/pax/more/newgrp/nice/nohup sit in the +4…+1 tail
or are absent), weighted by how many test purposes each gap plausibly
touches and whether the gap is silent (wrong answer) or loud (exit 2).

1. **pr header line format** (`cmds/pr/pr.go:878-898`) — POSIX-locale
   `"%s %s Page %d"` with `date "+%b %e %H:%M %Y"` vs the GNU/ISO
   centered header. Every paginated pr TP observes the header, so this
   single gap plausibly dominates the measured **pr +15**. Test-pinned
   in the wrong direction by `#TestPRDefaultPageStructure`.
2. **pr option/layout cluster** — `-p` absent, `-i` silent no-op,
   `-e`/`-i` not assumed for multi-column, `-e0` rejected, `-s` merge
   layout and truncation/512-width rules, clustered `-3d` rejected,
   `--` not protecting a `+` file, `-f` pause absent, merge diagnostics
   with a literal `\n` (`cmds/pr/pr.go` per-row citations above). Each
   is individually narrow but pr's TP count is large; together they
   cover the remainder of pr +15.
3. **od `-t` grammar** — concatenated type_strings (`-t dc`) and
   `fF`/`fD` rejected (`cmds/od/od.go:484-567`); plus option-order
   significance lost (`od.go:98-125`) and the XSI offset-operand gating
   (`od.go:82-89`, both directions). Directly TP-visible parse failures;
   the best candidates for the measured **od +5**.
4. **mkfifo `-m` symbolic omitted-who umask masking**
   (`cmds/mkfifo/mkfifo.go:80-85,180-216`) — `-m =rwx` under umask 077
   yields 777 instead of 700. mkfifo measured at **+5**; a
   mode-under-umask TP hits exactly this, and the sibling mkdir engine
   already implements the rule (fix by convergence).
5. **paste `-s` empty-file newline** (`cmds/paste/paste.go:257-261`) —
   drops the required newline-only output line; silent-wrong-output in a
   base utility whose other behavior is fully conformant (test-pinned in
   the wrong direction by `#TestPasteSerial`).
6. **mv prompt semantics** — declined `-i` exits 1 instead of 0
   (`cmds/mv/mv.go:194-197`, test-pinned wrong), the step-1 prompt for
   an unwritable destination on a terminal is absent (`mv.go:184-201`),
   and `lastOverride` cancels `-i` on bytes inside attached
   option-values (`mv.go:465-477`). mv is in the low tail today, but
   the exit-status gap is cheap to hit in any interactive-overwrite TP.
7. **mv cross-filesystem characteristic duplication** — atime never
   duplicated, uid/gid never attempted, setuid/setgid not conditionally
   stripped, FIFOs/devices byte-copied (`cmds/mv/mv.go:310-395`).
   Real defects, but PCTS rarely exercises cross-device renames in a
   single-filesystem test root — lower TP odds.
8. **nohup internal-error status 125 vs 127** (`cmds/nohup/nohup.go:40-45`)
   — Issue 7 requires 127 unconditionally; conformant only when the
   harness exports `POSIXLY_CORRECT`. Zero-cost to fix or to pin in the
   certification environment.
9. **more POSIX flags** — `-i`/`-t` rejected, `-p` misregistered as a
   boolean, `$MORE` ignored, no terminal mode (`cmds/more/more.go`).
   `more` is [UP]-optional and interactive TPs are few; low delta.
10. **pax surface** — unmatched pattern exits 0, `-b/-o/-t/-X/-H/-L`
    unregistered (three advertised in its own usage), `-i/-l/-n` and
    `-s`'s `p` silent no-ops, `-p` honors only `m`, `-a` silently loses
    the appended member, cpio write-only, `-s` uses Go regexp not BRE,
    `-d` ignored outside write mode, and one escaping member aborts the
    whole archive instead of diagnosing it and continuing with safe members
    (`cmds/pax/pax.go`, `modes.go`, `select.go`). Large gap count, but pax was not among
    the measured deltas; the silent no-ops (`-i -l -n`, `-a` data loss)
    still violate the repo's never-silent rule and deserve loud
    refusals ahead of full implementations.
11. **newgrp environment and supplementary-group handling** — `-l` changes
    argv0 and cwd but forwards `rc.Env` unchanged (`newgrp.go:171-182`,
    `spawn_unix.go:41-45`); the no-operand supplementary-list restore and
    add/delete rules never happen (`spawn_unix.go:61`); the password prompt
    goes to the tty rather than stderr. The `-l` gap is unprivileged and
    directly observable; supplementary-group gaps require privilege.
12. **nice child-priority race** — the child starts at `nice.go:179`, then
    `setPriority` runs at `nice.go:188`; a fast child can execute or exit at
    the original niceness. A priority-readback TP can observe it, but nice is
    in the low measured tail.
13. **pathchk filesystem-specific checks** — default PATH_MAX and NAME_MAX
    are hardcoded rather than queried from the underlying/containing
    filesystem, and byte-sequence validity is absent (`pathchk.go:60-94`).
    Silent wrong answers are possible on mounts whose constraints differ
    from the host defaults, though the ordinary certification filesystem is
    unlikely to expose the gap.
14. **ps** — default selection, `-a/-d/-t/-g` axes, `-u/-U` conflation,
    `-f`/`-l` column sets, `TT`/`COMMAND` headers, etime/time shapes,
    hardcoded pcpu, glued columns, silent `!linux` degradation
    (`cmds/ps/ps.go`, `process_other.go`). The largest gap count in
    this batch, but ps fails **identically in both arms** of the
    measurement (zero delta), so it ranks last for certification
    effort. The silent darwin degradation still contradicts the repo's
    own loud-refusal policy and is worth fixing on those grounds alone.

Cross-cutting note for fixes: five gaps above are **pinned by existing
tests asserting the nonconforming behavior** (`#TestPRDefaultPageStructure`,
`#TestPRFormFeedLowerEqualsUpper`, `#TestPRCustomSeparatorDoesNotTruncateColumns`,
`#TestPRMerge`, `#TestODShortTypeAliases`, `#TestPasteSerial`,
`#TestMvInteractiveRefusalContinuesAndFails`) — each fix must update its
pinning test in the same change.
