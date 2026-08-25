# POSIX Issue 7/2016 Go interface audit — batch 1

Scope is deliberately limited to `at`, `awk`, `basename`, `batch`, `cat`,
`chgrp`, `chmod`, `chown`, `cksum`, `cmp`, `comm`, `cp`, and `crontab`. This is
a source-and-behavior audit, not a transcription of help output. The normative
references are The Open Group Base Specifications Issue 7, 2016 Edition. GNU
extensions are mentioned only where they obscure or contradict the required
POSIX interface.

The classifications are fail-closed:

- `verified`: the required interface is present in executable source and is
  exercised by focused behavioral tests.
- `implementation_gap`: source or a focused test proves required behavior is
  absent or contradictory.
- `evidence_gap`: source appears compatible, but focused tests do not cover a
  required clause well enough to claim it.

The focused gate run for this audit was:

```text
go test ./cmds/at ./cmds/awk ./cmds/basename ./cmds/batch ./cmds/cat \
  ./cmds/chgrp ./cmds/chmod ./cmds/chown ./cmds/cksum ./cmds/cmp \
  ./cmds/comm ./cmds/cp ./cmds/crontab
```

It passed on 2026-08-24. A passing package test does not override a source-level
gap or a test that asserts non-POSIX output.

## Result summary

| Command | Overall classification | Decisive reason |
| --- | --- | --- |
| `at` | `implementation_gap` | Required `-m` is absent; empty/blank programs are rejected; time grammar/locale handling and removal-error status are wrong. |
| `awk` | `implementation_gap` | `LC_COLLATE` and `LC_NUMERIC` are not applied to the language engine. |
| `basename` | `evidence_gap` | Core transformation is covered, but null/`//` choices and locale/diagnostic behavior are not explicitly pinned. |
| `batch` | `implementation_gap` | Empty/blank programs are rejected and it is not equivalent to `at -q b -m now`; access, mail, locale, and timezone semantics are absent. |
| `cat` | `evidence_gap` | Required byte copying and `-u` are covered; repeated `-` and arbitrary file-type behavior are not fully covered. |
| `chgrp` | `implementation_gap` | Recursive `-H`/`-L`, last-option-wins, and same-ID `chown()` effects are absent. |
| `chmod` | `evidence_gap` | Main mode grammar is covered; timestamp, set-ID edge cases, and recursive error consequences lack focused evidence. |
| `chown` | `implementation_gap` | Recursive `-H`/`-L`, last-option-wins, and same-ID `chown()` effects are absent. |
| `cksum` | `evidence_gap` | Source appears compatible, but special-file and mixed valid/missing continuation evidence is incomplete. |
| `cmp` | `implementation_gap` | Default POSIX output requires `char`; implementation and test require `byte`. |
| `comm` | `implementation_gap` | Every non-C/POSIX locale except two ISO-8859-1 aliases is rejected, and the provider is unavailable off Linux amd64/arm64. |
| `cp` | `implementation_gap` | Declining `-i` returns failure; affirmative matching ignores locale; new recursive-directory mode loses the umask. |
| `crontab` | `implementation_gap` | `%` splitting, invocation-independent mandated job defaults, mail, and XSI access control are absent. |

## `at`

Normative source: [Open Group Issue 7/2016 `at`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/at.html).

Classification: `implementation_gap`.

Required interface:

- Synopses: `at [-m] [-f file] [-q queuename] -t time_arg`;
  `at [-m] [-f file] [-q queuename] timespec...`; `at -r at_job_id...`;
  `at -l -q queuename`; and `at -l [at_job_id...]`.
- Required flags are `-f file`, `-l`, `-m`, `-q queuename`, `-r`, and
  `-t time_arg`. `-f`, `-q`, and `-t` require option-arguments. The five
  synopsis families constrain combinations and arity; `-r` needs one or more
  IDs, while `-l` accepts zero or more IDs except for its queue-only form.
- `timespec` operands are space-concatenated. POSIX-locale special tokens
  include `midnight`, `noon`, `now`, `today`, `tomorrow`, case-insensitive
  locale month/day names and AM/PM, case-insensitive `utc`, `+ number` plus singular/plural
  `minute|hour|day|week|month|year`, and `next` plus one of those units.
  The grammar permits tokens to be adjacent where they remain unambiguous, and
  a timezone name is part of `time`, before an optional `date` or increment.
  `time_arg` has the `touch -t` form `[[CC]YY]MMDDhhmm[.SS]`.
- Unless `-f` is given, stdin is a text file of shell command language. The
  job runs in a separate shell/process group without a controlling terminal,
  retaining submission environment, cwd, umask, and implementation-defined
  execution attributes. `SHELL` selects an allowed interpreter policy.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `LC_TIME`,
  `SHELL`, and `TZ`; `NLSPATH` is XSI. A timezone in `timespec` overrides
  `TZ`.
- On submission stdout is empty except implementation-defined terminal
  prompts; stderr is `job %s at %s\n` using `date +"%a %b %e %T %Y"` shape.
  `-l` writes `%s\t%s\n` per job to stdout in the user's timezone.
  Diagnostics go to stderr. `-m` mails completion even without job output;
  without it, unredirected job stdout/stderr is mailed when present.
- Exit 0 means submission, removal, or listing succeeded; greater than zero
  means error, and on error the job is not scheduled, removed, or listed.
  Under XSI, `at.allow`/`at.deny` impose the specified allow-first access rule.

Source comparison:

- `cmds/at/at.go` registers `-f`, `-l`, `-r`, `-q`, and `-t`, validates a
  one-letter lowercase queue, preserves shell text/environment/cwd/umask, and
  routes listing to stdout and confirmation/diagnostics to stderr. It has no
  `-m` parser entry or mail state at all. It also trims and rejects empty or
  blank shell programs, although POSIX permits empty shell text.
- `pkg/schedule/export.go` implements much of the POSIX-locale grammar and
  `touch -t`, but `splitIncrement` only accepts `+ N unit`; it rejects required
  `next unit`. Its month, weekday, and AM/PM tables are fixed English tokens,
  so `LC_TIME` does not determine accepted names. It recognizes
  case-insensitive `utc` only as the final whitespace-separated field, not in
  its required position within `time` before an optional date. The
  whitespace-based parser also rejects required unambiguous adjacent-token
  forms illustrated by the standard, such as `17 utc+ 30minutes` and
  `8:15amjan24`.
- `cmds/at/access_unix.go` implements the XSI allow/deny precedence on Unix;
  `cmds/at/access_windows.go` unconditionally allows access.
- `cmds/at/at.go` accepts some cross-family combinations as extensions; these
  are not POSIX defects when `--` shields operands from option parsing.
- `removeJobs` writes `no job` for an unknown ID but still returns 0. That ID
  was not successfully removed, so the required success/error status split is
  violated. `listJobs` formats the stored `time.Time` directly instead of
  adjusting it to `TZ` from the listing invocation. Both listing and submission
  format dates with a fixed English Go layout, so `LC_TIME` does not determine
  the format and contents of written date strings.

Behavioral evidence: `cmds/at/at_test.go` covers create/list/remove, queue
filtering, `-t`, stdin and `-f`, shell-program/cwd/environment/umask capture,
diagnostics, errors, and a substantial time grammar. `TestParseLicensedAtGrammar`
does not cover `next`. `cmds/at/at_daemon_unix_test.go` covers retained
submission context; `cmds/at/access_unix_test.go` covers allow/deny precedence.
`TestAtRemoveNonexistent` explicitly requires the contradictory status 0.
There is no `-m`, mail-delivery, locale-name, UTC-before-date, adjacent-token,
written-`LC_TIME`, listing-timezone, or empty/blank shell-program test.

Missing features: `-m`; completion/output mail delivery; `next` increments;
locale-derived names and output; the required UTC placement and adjacent-token
grammar; nonzero status for unsuccessful removal; listing timezone adjustment;
and Windows XSI access control.

## `awk`

Normative source: [Open Group Issue 7/2016 `awk`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html).

Classification: `implementation_gap`.

Required interface:

- Synopses: `awk [-F sepstring] [-v assignment]... program [argument...]` and
  `awk [-F sepstring] -f progfile [-f progfile]... [-v assignment]...
  [argument...]`.
- `-F sepstring`, repeatable `-f progfile`, and repeatable `-v assignment`
  each require an option-argument. `-F` is equivalent to `-v FS=sepstring`
  subject to the specified ordering latitude. With no `-f`, exactly one first
  operand supplies the program. Remaining operands may intermix files and
  `name=value` assignments; those assignments occur immediately before the
  following file (after `BEGIN`, and before `END` if last).
- Special `-` means stdin as a file operand or `progfile`. With no input file,
  stdin supplies text records. A valid program with neither patterns nor
  actions reads no input and returns zero. Program and input are text files.
- The effect is execution of the specified POSIX awk language: ordered
  pattern/action processing, records and fields, built-ins, EREs, numeric and
  string conversions, I/O, and program-controlled `exit` status.
- Environment: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`,
  `LC_NUMERIC`, `PATH`, and XSI `NLSPATH`; every environment variable is
  exposed through `ENVIRON`.
- Stdout and output files are program-defined. Stderr is diagnostics only.
  Exit 0 means all inputs processed; greater than zero means error, unless
  changed by `exit expression`. An inaccessible file operand must diagnose and
  terminate without further action.

Source comparison:

- `cmds/awk/awk.go` has non-interspersed `-F`, repeatable `-f`, repeatable
  `-v`, validates `-v` names, handles `-f -`, preserves operand assignment
  order through GoAWK, passes the invocation environment to `ENVIRON`, and
  returns interpreter status.
- The custom ERE seam in `cmds/awk/awk.go` consults only resolved `LC_CTYPE`.
  No code resolves `LC_COLLATE` for regex ranges or string comparisons, and no
  code resolves `LC_NUMERIC` for input, conversions, or formatted output.
  Passing environment pairs to GoAWK is not implementation of those language
  semantics.

Behavioral evidence: `cmds/awk/awk_test.go` covers basic execution, POSIX
numeric format fixes, ERE behavior, program files, and operand spelling;
`cmds/awk/awk_routing_test.go` exercises difficult ERE routing;
`cmds/awk/awk_locale_test.go` exercises the `LC_CTYPE` regex provider and its
lifecycle. There is no focused `LC_COLLATE` string-comparison/range test or
`LC_NUMERIC` conversion/format test, matching the missing source wiring.

Missing features: POSIX locale collation in string comparisons and relevant
ERE constructs, and POSIX locale numeric input/conversion/output behavior.
Full language conformance beyond the focused GoAWK tests remains an additional
evidence gap, but is not promoted to a confirmed implementation gap here.

## `basename`

Normative source: [Open Group Issue 7/2016 `basename`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html).

Classification: `evidence_gap`.

Required interface:

- Synopsis: `basename string [suffix]`; no options. Arity is one or two
  operands. Stdin is unused.
- Strip trailing slashes, reduce all-slash strings to `/`, remove through the
  final remaining slash, then remove `suffix` only when it is a proper suffix
  and not the whole result. A null `string` may produce null or `.`; handling
  of exactly `//` is implementation-defined. Those are the special cases.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI
  `NLSPATH`.
- Stdout is exactly `%s\n` with the resulting string; stderr is diagnostics
  only. Exit 0 is success and greater than zero is error. No output files or
  other effects are required.

Source comparison: `cmds/basename/basename.go` implements the ordered pathname
transformation in `base`, enforces one/two operands for the POSIX form, writes
one newline, detects short writes, and leaves stdin untouched. GNU `-a`, `-s`,
and `-z` are extensions; their presence is not evidence for the POSIX form.
The source chooses null output for null input and processes `//` to `/`, both
allowed choices.

Behavioral evidence: `cmds/basename/basename_test.go` covers ordinary paths,
suffix removal/non-removal, trailing/all slashes, arity diagnostics, and output
write errors. It does not explicitly pin null input, exactly `//`, stdin
non-consumption, or locale-sensitive argument/diagnostic behavior.

Missing evidence: focused tests for the allowed null and `//` choices, stdin
non-use, and relevant locale behavior. No required transformation defect was
found.

## `batch`

Normative source: [Open Group Issue 7/2016 `batch`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/batch.html).

Classification: `implementation_gap`.

Required interface:

- Synopsis is exactly `batch`; there are no options, option-arguments, or
  operands. Stdin is a text file of shell command language.
- Its effect is equivalent to `at -q b -m now`: submit without time constraints
  to reserved batch queue `b`, use implementation-defined load scheduling,
  and mail completion even with no output. The XSI `at.allow`/`at.deny` access
  rules apply.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, `LC_TIME`,
  `SHELL`, `TZ`, and XSI `NLSPATH`.
- Stdout is empty except optional terminal prompts. Successful submission
  writes `job %s at %s\n` to stderr with the specified date shape;
  diagnostics also use stderr. Exit 0 is success, greater than zero error, and
  error must not schedule the job.

Source comparison: `cmds/batch/batch.go` trims and rejects empty or blank shell
programs, although POSIX permits empty shell text. It retains environment, cwd,
and umask, stores a one-shot job, emits confirmation on stderr, and returns
nonzero on errors. It schedules `now + 1 second` with empty queue rather than
implementing the required `at -q b -m now` equivalence; it has no
mail/completion state and never invokes `checkAtAccess`. Its `time.Now()` and
fixed English Go layout ignore invocation `TZ` and `LC_TIME` when writing the
confirmation date. Its acceptance of `-f` and extra operands is an extension,
not a POSIX defect, when `--` shields operands.

Behavioral evidence: `cmds/batch/batch_test.go` covers stdout silence, stderr
confirmation format, the contradictory rejection of empty stdin, persisted
shell text/context, and unknown flags. The persisted-job test asserts
`Kind == "at"` but does not require queue `b`, mail behavior, access checks,
load scheduling, operand rejection, or locale/timezone output.

Missing features: acceptance and scheduling of empty or blank shell programs;
reserved queue `b`; `-m`-equivalent notification; XSI access control;
load-based scheduling; and `LC_TIME`/`TZ`-correct confirmation dates.

## `cat`

Normative source: [Open Group Issue 7/2016 `cat`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html).

Classification: `evidence_gap`.

Required interface:

- Synopsis: `cat [-u] [file...]`; `-u` has no option-argument and requires
  bytes to be written without delay.
- Zero or more file operands are accepted. With none, stdin is used. Each `-`
  means stdin at that point; multiple `-` occurrences continue on the same
  stream without closing/reopening it. Inputs may be any file type.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI
  `NLSPATH`.
- Stdout is only the bytewise concatenation in operand order. The
  implementation may reject a regular output file that is also an input.
  Stderr is diagnostics only. Exit 0 means every input was output; greater
  than zero means error.

Source comparison: `cmds/cat/cat.go` pre-parses `-u`, preserves one shared
`rc.In` for every `-`, streams files in order, continues after open errors,
tracks write/flush failures, and detects identical regular input/output files.
The other flags are GNU extensions and do not establish `-u` behavior.

Behavioral evidence: `cmds/cat/cat_test.go` covers no-operand stdin, one `-`,
file/stdin ordering, `-u` flushing before EOF, continuation and status after a
missing file, identical files, and write errors. `cmds/cat/cat_fifo_test.go`
covers FIFO/symlink streaming. No focused test uses multiple `-` operands or
the broader required set of arbitrary file types.

Missing evidence: repeated `-` on one stream, additional special-file types,
and relevant locale/diagnostic behavior. No required source defect was found.

## `chgrp`

Normative source: [Open Group Issue 7/2016 `chgrp`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html).

Classification: `implementation_gap`.

Required interface:

- Synopses: `chgrp [-h] group file...` and
  `chgrp -R [-H|-L|-P] group file...`. Required flags are `-h`, `-H`, `-L`,
  `-P`, and `-R`, with no option-arguments. `group` is exactly one operand and
  at least one `file` is required.
- `group` is a database name or numeric ID; if a numeric spelling is also a
  database name, name lookup wins. Each file's group changes while owner is
  retained. `-h` acts on a command-line symlink. With `-R`, `-H` follows a
  command-line directory symlink, `-L` follows all directory symlinks, and
  `-P` changes symlinks without following them. Multiple `HLP` options are not
  errors and the last wins. Successful ownership change has the specified
  set-ID clearing consequences.
- Stdin and stdout are unused; stderr is diagnostics only. Environment is
  `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`. Exit 0 means
  all requested changes occurred; greater than zero means error.

Source comparison: `cmds/chgrp/chgrp.go` parses the required flags and arity;
`cmds/chgrp/chgrp_unix.go` performs name-before-number lookup and uses
`Chown`/`Lchown`. However, recursion uses `filepath.WalkDir`, which never
descends through a symlink directory. Merely calling `chgrpOne(..., follow=true)`
on an entry cannot implement `-H`/`-L` hierarchy traversal. Flag order is also
collapsed to booleans: `noTraverse` forces `derefNever` regardless of whether
`-L` appeared later, and `-L` always outranks `-H`. `cmds/chgrp/chgrp_windows.go`
fails every ownership operation.
`chgrpOne` returns early when the existing GID equals the target. POSIX
requires actions equivalent to `chown()` for each file; bypassing that call
can bypass its set-user-ID/set-group-ID clearing consequence for an
unprivileged successful invocation.

Behavioral evidence: `cmds/chgrp/chgrp_test.go` covers ordinary/recursive
changes, required arity, name errors, non-recursive dereference and `-h`.
There is no test that places a child below a command-line or encountered
symlink directory for `-H`/`-L`, nor a required last-option-wins matrix.

Missing features: recursive `-H` and `-L` descent, order-sensitive `HLP`
resolution, same-ID `chown()` side effects, and POSIX ownership behavior on
Windows.

## `chmod`

Normative source: [Open Group Issue 7/2016 `chmod`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html).

Classification: `evidence_gap`.

Required interface:

- Synopsis: `chmod [-R] mode file...`; only required flag is `-R`, with no
  option-argument. Exactly one mode and one or more files are required.
- `mode` is a non-negative octal integer or the formal symbolic grammar:
  comma-separated clauses; optional `u|g|o|a`; one or more `+|-|=` actions;
  permissions `rwxXs` and permission copies `u|g|o`; XSI additionally requires
  `t`. Omitted `who` is filtered by the process umask for setting and clearing
  as specified. `X`, empty permission lists, ordered actions, set-ID, sticky,
  and absolute octal semantics are special cases.
- Each named file, recursively with `-R`, has its requested mode bits changed;
  a successful change updates the file status-change timestamp. Alternate ACL
  effects and listed set-ID cases are implementation-defined.
- Stdin and stdout are unused; stderr is diagnostics only. Environment is
  `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`. Exit 0 means
  all requested changes occurred; greater than zero means error.

Source comparison: `cmds/chmod/chmod.go` parses `-R`, rescues dash-leading
symbolic modes, and implements octal and symbolic grammar; `cmds/chmod/chmod_unix.go`
applies umask-aware changes recursively and delegates timestamp updates to
`chmod(2)`. GNU traversal/output flags are extensions. `cmds/chmod/chmod_windows.go`
fails loudly because Windows has no matching mode model.

Behavioral evidence: `cmds/chmod/chmod_test.go` extensively covers symbolic
operators, octal modes, omitted-who umask behavior, `X`, permission copying,
recursion, symlink policies, arity/errors, and statuses. It does not directly
assert status-change timestamp updates, the full set-ID/sticky matrix, behavior
after a failure among multiple operands, or locale diagnostics.

Missing evidence: timestamp update, remaining set-ID/sticky edge cases,
multi-operand error continuation/consequences, and locale behavior. The Unix
required parser/effect has no confirmed defect; the Windows implementation is
not POSIX-capable.

## `chown`

Normative source: [Open Group Issue 7/2016 `chown`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html).

Classification: `implementation_gap`.

Required interface:

- Synopses: `chown [-h] owner[:group] file...` and
  `chown -R [-H|-L|-P] owner[:group] file...`. The standard displays the
  optional group as `[ : group ]`. Required flags are `-h`, `-H`, `-L`, `-P`,
  and `-R`, without option-arguments. One owner specification and at least one
  file are required.
- Owner and optional group accept database names or numeric IDs, with database
  name lookup winning for numeric-looking names. Omitted group is unchanged.
  `-h`, recursive `HLP`, last-option-wins, hierarchy behavior, and set-ID
  clearing parallel `chgrp`.
- Stdin and stdout are unused; stderr is diagnostics only. Environment is
  `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`. Exit 0 means
  every requested change occurred; greater than zero means error.

Source comparison: `cmds/chown/chown.go` parses the required flags and arity;
`cmds/chown/chown_unix.go` parses names/IDs and calls `Chown`/`Lchown`. As in
`chgrp`, recursive traversal is `filepath.WalkDir`, so `-H` and `-L` can change
a symlink referent itself but cannot descend through its referenced directory.
Boolean precedence also makes `-P` win regardless of later `-H`/`-L`, and
`-L` win over `-H` regardless of source order. `cmds/chown/chown_windows.go`
fails all ownership operations.
`chownOne` also returns before `chown()` when the requested IDs already match,
which can omit the standard's set-ID clearing consequence of the required
equivalent call.

Behavioral evidence: `cmds/chown/chown_test.go` covers parsing, ordinary and
recursive changes, errors, non-recursive dereference, and `-h`. It has no
recursive symlink-directory descent assertion or last-option-wins matrix.

Missing features: recursive `-H`/`-L` descent, ordered `HLP` resolution, and
same-ID `chown()` side effects, and POSIX ownership behavior on Windows.

## `cksum`

Normative source: [Open Group Issue 7/2016 `cksum`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cksum.html).

Classification: `evidence_gap`.

Required interface:

- Synopsis: `cksum [file...]`; there are no required options or
  option-arguments and zero or more files are allowed.
- With no operands stdin is used. An implementation may treat a `-` operand as
  stdin; Bashy chooses to do so. Inputs may be any file type. For each input it
  computes the specified Ethernet-polynomial CRC over file bits followed by
  the least-significant-octet-first length encoding, complements the remainder,
  and reports the octet count.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI
  `NLSPATH`.
- Per successful file stdout is `%u %d %s\n`; with no operand, omit the pathname
  and its leading space. Stderr is diagnostics only. Exit 0 means all files
  were processed; greater than zero means error.

Source comparison: the default `crc` path in `cmds/cksum/cksum.go` is distinct
from GNU digest extensions. `cksumOperand` implements the required polynomial
table update, length folding, complement, and octet count; `openOperand`
implements the chosen `-` behavior. Output/error aggregation returns failure
if any operand or output write fails.

Behavioral evidence: `cmds/cksum/cksum_test.go::TestCKSumStdinAndFiles` pins
known CRCs and both pathname forms; `TestCKSumErrors` covers missing inputs but
is not focused mixed valid/missing continuation evidence; and
`TestCKSumReportsStandardOutputWriteError` covers output failure. There is no
focused special-file test or mixed valid/missing continuation test. The many
algorithm/check-mode tests concern extensions and were not used to infer POSIX
support.

No source contradiction was found in the POSIX default interface. Locale
diagnostic translation remains a project-wide certification concern, but
special-file and mixed valid/missing continuation evidence is still missing.

## `cmp`

Normative source: [Open Group Issue 7/2016 `cmp`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cmp.html).

Classification: `implementation_gap`.

Required interface:

- Synopsis: `cmp [-l|-s] file1 file2`; exactly two operands. `-l` and `-s` are
  mutually exclusive and take no option-arguments.
- Either file may be `-` for stdin. If both denote stdin, or both denote the
  same FIFO/block/character special file, results are undefined.
- Default stdout on the first difference is
  `%s %s differ: char %d, line %d\n`. `-l` writes
  `%d %o %o\n` for every differing byte. `-s` writes no comparison output.
  Prefix-length differences diagnose `cmp: EOF on %s%s\n` on stderr except
  that `-s` error diagnostics are unspecified. Other stderr is diagnostics.
- Environment: `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI
  `NLSPATH`. Exit 0 means identical, 1 different (including proper-prefix),
  and greater than 1 error.

Source comparison: `cmds/cmp/cmp.go` parses `-l`/`-s`, rejects their
combination, handles stdin, emits octal `-l` rows, handles EOF, and returns
0/1/2. But `cmpFirstDiff` deliberately emits GNU wording
`differ: byte %d, line %d`; POSIX requires `char`. The parser also accepts GNU
one-file/default-stdin, skip operands, and other flags, but those extensions do
not repair the required output mismatch.

Behavioral evidence: `cmds/cmp/cmp_test.go::TestCmpDiffer` explicitly expects
`byte`, proving the mismatch. `TestCmpVerbose`, `TestCmpSilent`, `TestCmpEOF`,
`TestCmpIdentical`, `TestCmpStdinAndSkips`, `TestCmpRejectsRepeatedStandardInput`,
and `TestCmpErrors` otherwise cover the required modes and statuses.

Missing feature: the exact POSIX-locale default `char` output token.

## `comm`

Normative source: [Open Group Issue 7/2016 `comm`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/comm.html).

Classification: `implementation_gap`.

Required interface:

- Synopsis: `comm [-123] file1 file2`; exactly two operands. `-1`, `-2`, and
  `-3` independently suppress their columns and take no arguments.
- Each operand names a sorted text file; one may be `-` for stdin. Both stdin,
  or the same FIFO/block/character special file, produces undefined results.
- Input ordering and merge comparison use `LC_COLLATE`. Equal-collating but
  byte-distinct lines may be treated equal or distinct subject to the stated
  secondary-order rules.
- Environment: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and
  XSI `NLSPATH`.
- Stdout contains unique-file1, unique-file2, and common lines with zero, one,
  or two tab leads after accounting for suppressed preceding columns; all
  suppressed means no output. Stderr is diagnostics only. Exit 0 means inputs
  were output as specified; greater than zero means error.

Source comparison: `cmds/comm/comm.go` pre-parses clustered `-123`, enforces
two operands, uses one stdin, streams three columns, checks and reports
I/O/output failures, and returns nonzero on failures. C/POSIX use the required
byte-order fast path. For every other `LC_COLLATE`, however, it calls
`pkg/collate.Open`; that provider accepts only the two `de_DE` ISO-8859-1
aliases and is implemented only on Linux amd64/arm64. Consequently `comm`
returns status 2 instead of comparing under any other valid installed locale,
and every non-C/POSIX locale fails on other platforms. Extra GNU flags are
separable extensions.

Behavioral evidence: `cmds/comm/comm_test.go` covers every suppression
combination, exact tab layout, duplicate/empty lines, stdin in either position,
arity and file errors, streaming, sort diagnostics, and status. It has no
failing stdout-writer test and no injected read-error test, despite source Flush
handling.
`TestCommUsesInvocationCollatorForMergeAndOrderChecks`,
`TestCommLocaleInitFailsBeforeInputOpen`, and `TestCommCAndPOSIXBypassCollator`
cover routing, the deliberately narrow locale provider, and its failure
behavior.

Missing feature: `LC_COLLATE` operation for valid installed locales other than
the two accepted ISO-8859-1 aliases, including all non-C/POSIX locale operation
off Linux amd64/arm64. Failing-output-writer and injected-read-error tests also
remain evidence gaps.

## `cp`

Normative source: [Open Group Issue 7/2016 `cp`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html).

Classification: `implementation_gap`.

Required interface:

- Synopses: `cp [-Pfip] source_file target_file`;
  `cp [-Pfip] source_file... target`; and
  `cp -R [-H|-L|-P] [-fip] source_file... target`.
  Required flags are `-f`, `-H`, `-i`, `-L`, `-P`, `-p`, and `-R`; none has
  an option-argument. `HLP` are mutually exclusive but multiple occurrences
  are valid and the last wins. There is at least one source and exactly one
  final target; multiple sources require an existing directory target.
- A `-` source or target is a literal filename, never stdin/stdout. Stdin is
  used only for `-i` responses. `-i` prompts on stderr before an existing
  non-directory destination and copies only on a locale-defined affirmative
  response. `-f` unlinks/retries only after opening an existing destination
  fails.
- Non-recursive sources follow symlinks unless `-P`; recursive `-H`, `-L`, and
  `-P` have their specified command-line/all/physical meanings. Directories,
  regular files, symlinks, FIFOs, and other supported file types follow the
  ordered creation/copy/error rules. Newly created recursive directories use
  source mode modified by umask without `-p`, temporarily OR'd with `S_IRWXU`.
- `-p` preserves atime/mtime, owner/group, mode and set-ID bits with the stated
  failure and set-ID-clearing rules. Existing same-file sources are not copied;
  errors diagnose and processing continues where required.
- Environment: `LANG`, `LC_ALL`, `LC_COLLATE`, `LC_CTYPE`, `LC_MESSAGES`, and
  XSI `NLSPATH`; the locale controls affirmative matching and diagnostics.
- Stdout is unused. Prompts and diagnostics use stderr. Exit 0 means all files
  copied successfully; greater than zero means error. Partial trees and
  incorrect metadata are permitted consequences of premature termination.

Source comparison:

- `cmds/cp/cp.go` parses every required flag, enforces arity/target forms, and
  `resolveDereferenceMode` preserves the last `HLP` option. Copy paths cover
  ordinary files, directories, symlinks and platform special files; `-p`
  delegates atime, owner, mode, and time handling to platform helpers.
- `copyFile` sets `c.failed = true` when an `-i` response declines. A decline
  is the specified no-copy branch, not an error diagnostic; the implementation
  therefore returns 1 where conforming `cp` should successfully continue.
  `cmds/cp/cp_test.go::TestCpInteractiveDecline` locks in that status 1.
- `confirm` accepts only hard-coded `y`, `Y`, or case-insensitive `yes`; it does
  not evaluate the `yesexpr` ERE from `LC_MESSAGES` using `LC_COLLATE` and
  `LC_CTYPE`.
- `copyDir` initially creates a directory with a maskable mode, but after
  population, when `-p` is absent, calls `os.Chmod(..., fi.Mode().Perm())`.
  That restores source permissions without applying the invoking umask,
  contrary to the required final mode.

Behavioral evidence: `cmds/cp/cp_test.go` broadly covers the three forms,
required flags, last-option dereference matrices, preservation, errors, and
prompts; `cmds/cp/cp_fifo_unix_test.go` covers recursive FIFO/socket behavior.
There is no locale-affirmative or recursive-directory-umask regression test.

Missing features: successful status/continuation after declining `-i`,
locale `yesexpr` matching, and the correct umask-filtered final mode for a
new recursive destination directory without `-p`.

## `crontab`

Normative source: [Open Group Issue 7/2016 `crontab`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/crontab.html).

Classification: `implementation_gap`.

Required and conditional interface:

- Base synopsis is `crontab [file]`: zero or one file; with no operand stdin
  provides the replacement table. The standard assigns no special meaning to
  a `-` file operand: it remains a pathname. The User Portability option
  condition adds
  `crontab [-e|-l|-r]`; these mutually exclusive flags take no arguments and
  accept no file operand.
- A table is text with five blank-separated schedule fields plus command.
  Fields support `*`, numbers, inclusive `N-M` ranges, and comma lists with the
  specified month/day-of-month/day-of-week matching rule. Blank lines and a
  first-nonblank `#` are ignored.
- In the command field, unescaped `%` becomes newline. Only the portion before
  the first such newline is executed by `sh`; remaining lines become command
  stdin. Backslash makes the following character literal. These are required
  special tokens, not extensions.
- Scheduled execution supplies a default environment containing at least
  `HOME`, `LOGNAME`, a `PATH` that finds standard utilities, and `SHELL` naming
  `sh`, independent of those values at installation. Unredirected stdout and
  stderr are mailed to the user.
- Environment affecting the utility: `EDITOR` for `-e` (default `vi`),
  `LANG`, `LC_ALL`, `LC_CTYPE`, `LC_MESSAGES`, and XSI `NLSPATH`. Under XSI,
  `cron.allow`/`cron.deny` enforce the specified allow-first access rule.
- `-l` writes the entry to stdout; other successful forms are silent. Stderr
  is diagnostics only. Exit 0 is success and greater than zero error; errors
  must not submit, remove, edit, or list the entry.

Source comparison:

- `cmds/crontab/crontab.go` parses zero/one file plus mutually exclusive
  `-e/-l/-r`; installation is atomic after parse/schedule validation and
  successful install is silent. It treats a `-` file operand as stdin, so a
  POSIX file operand whose pathname is `-` cannot be installed, even after an
  option terminator.
- `parseCronTab` and `splitCronLine` preserve the entire sixth field but never
  interpret unescaped `%` or backslash escapes and never create command stdin.
- `installCronLines` stores the invocation's whole environment and selects
  `SHELL` from it, plus invocation cwd/umask. POSIX does not require an
  otherwise-clean environment, but it does require default `HOME`, `LOGNAME`,
  `PATH`, and `SHELL=sh` values that are independent of their values when
  `crontab` is invoked; the stored invocation values contradict that mandate.
- No `cron.allow`/`cron.deny` access check or output-mail delivery exists.
  `editCron` reads process-global `os.Getenv("EDITOR")` rather than
  `rc.Getenv`, so embedded Bashy invocations can ignore their invocation
  environment; it also wires the editor to process-global stdio.

Behavioral evidence: `cmds/crontab/crontab_test.go` covers install/list/remove,
stdin and file replacement, comments, round-trip text, conflicting modes,
atomic rejection, schedule validation, whitespace, silence, and persistence.
`TestCrontabPersistsShellProgramAndContext` affirmatively expects captured
invocation environment/umask, exposing the environment contradiction. There
is no `%`/backslash stdin split, invocation-independent mandated-default
environment, mail, access, or `EDITOR` RunContext test.

Missing features: command `%`/backslash translation and command stdin;
required independent default job environment and `sh`; output/error mail;
XSI access control; literal pathname semantics for file operand `-`; and
invocation-scoped `EDITOR`/stdio for `-e`.

## Confirmed-gap ranking by likely VSC-PCTS TP impact

This ranking counts only source- or behavior-confirmed gaps. It does not turn
an `evidence_gap` into a predicted failure.

1. **`crontab` command parsing and execution context** — `%` command/stdin
   splitting and the mandated independent `HOME`/`LOGNAME`/`PATH`/`SHELL`
   environment are central functional cases and can affect many execution TPs.
2. **`awk` locale semantics** — absent `LC_COLLATE` and `LC_NUMERIC` wiring can
   fan out across comparisons, regex ranges, numeric parsing, conversions, and
   formatted output.
3. **`at` required `-m`, status, and `timespec` grammar** — a required parser
   flag fails immediately; unsuccessful removal returns success; and `next`
   plus locale time tokens affect multiple scheduling cases.
4. **`chgrp` and `chown` recursive symlink policies** — both commands lose
   required `-H`/`-L` descent and last-option-wins behavior, likely multiplying
   failures across traversal matrices.
5. **`cp` interactive and recursive-directory semantics** — declined `-i`
   status, locale affirmative matching, and umask loss are independent likely
   TP failures.
6. **`comm` locale coverage** — nearly every non-C/POSIX `LC_COLLATE` is
   rejected rather than used for comparison, which can affect every
   locale-sensitive merge and ordering case.
7. **`batch` equivalence contract** — queue `b`, forced mail, access control,
   and load scheduling are missing, though batch coverage is likely narrower
   than `at`/`crontab`.
8. **`cmp` exact default output token** — highly deterministic and likely one
   direct output-format failure (`byte` versus `char`), but with limited fanout.

`basename`, `cat`, and `chmod` are not ranked because their open items are
evidence gaps, not confirmed failures. `cksum` has no confirmed
required-interface gap in this audit.
