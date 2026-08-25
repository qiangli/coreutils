# `ps` Issue 758 POSIX.1-2008 Issue 7 Audit

Scope: Open Group POSIX.1-2008 Issue 7, 2016 Edition `ps`, audited against
repository baseline `9982454`. GNU and BSD behavior was considered only for
non-conflicting extensions; it is not conformance evidence.

## Result

The implementation has substantially stronger focused evidence, but the
canonical interface row remains `partial`: no full Issue 7 harness run was
performed, and focused tests and cross-builds are not a certification result.
The confirmed observable defects fixed include:

- Linux process start time now comes from `/proc/<pid>/stat` field 22 plus
  `/proc/stat` `btime`, using the kernel `AT_CLKTCK` auxiliary-vector value.
  The dependency's `/proc/<pid>` inode-change timestamp is no longer presented
  as process start time. If boot time or clock ticks is unavailable, start,
  elapsed time, and CPU-derived values remain unavailable instead of using a
  guessed constant.
- `tty` output now retains the device line name used by `who` (for example,
  `tty04`) while `-t` still accepts `tty04`, `/dev/tty04`, and the XSI short
  identifier `04`. Linux UNIX98 PTY majors 136 through 143 are decoded with
  their major-range offset; nonstandard devices resolve through sysfs and are
  left unavailable if the kernel supplies no name.
- A successfully read Linux wait channel containing `0` is rendered as the
  required blank running field. A failed read is separately represented by a
  hyphen.
- When `COLUMNS` is unset, null, or invalid, a terminal-backed standard output
  now supplies its live width. A valid invocation-local `COLUMNS` value still
  overrides it.
- Live Linux rows now require a valid per-process stat record, keep real and
  effective IDs unavailable until `/proc/<pid>/status` supplies them, and read
  `/proc/<pid>/cmdline` directly so an empty `argv[0]` and empty arguments are
  not collapsed by the portable enumerator.

The old non-Linux provider supplied no SID, controlling terminal, real IDs,
CPU time, or Linux enrichment, yet could silently select and label partial
rows. Live `ps` now fails with status 1 and an explicit Linux-only diagnostic
on non-Linux targets. Hermetic formatting and selection remain portable and
cross-compiled. This is a deliberate fail-closed disposition, not an
approximation.

## Normative Coverage

| Area | Audited disposition |
| --- | --- |
| Synopsis and operands | No operands. Empty required option-arguments and unexpected operands diagnose with status 2 before enumeration. |
| Selection | `-A`, `-a`, `-d`, `-e`, `-g`, `-G`, `-p`, `-t`, `-u`, and `-U` have end-to-end evidence. Criteria combine by inclusive OR; any selection option suppresses the default same-effective-UID and same-controlling-terminal rule. `-g` selects session IDs, not process groups. |
| Non-selection options | `-f`, `-l`, `-n`, and `-o` do not change selection. `-n` designates a namelist but is observably a no-op because Linux procfs uses none. Repeated `-o` values preserve command-line field order. |
| `-x` | Not present in the official 2016 Issue 7 synopsis or option list. It remains rejected; accepting it would be an extension, not POSIX closure. |
| Standard layouts | XSI default, full, long, and combined full-long headings/order are pinned. Full output uses arguments, brackets an unavailable argument vector, and all standard layouts mark zombies defunct. |
| `-o` fields | All required POSIX names and default headers are pinned: `ruser`, `user`, `rgroup`, `group`, `pid`, `ppid`, `pgid`, `pcpu`, `vsz`, `nice`, `etime`, `time`, `tty`, `comm`, and `args`. Field order, blank/comma separators, header overrides containing blanks, null headers, and the all-null no-header rule are covered. |
| Identities | User/group names are resolved for selectors and textual fields; numeric selectors bypass name lookup. Failed or empty textual lookups fall back to decimal IDs. Explicit `uid`/`gid` and the extra long-layout fields are documented implementation extensions. |
| Linux live data | Proc fixtures and a Linux-only live enumerator test cover PID/PPID/PGID/SID, real/effective IDs, argv, state, flags, priority, nice, CPU, virtual size, page size, address, wait channel, terminal, and start time. Snapshot races are tolerated by omitting vanished processes. |
| Unavailable data | Required portable optional fields use a hyphen when the provider cannot supply a meaningful value. Linux clock ticks come from `AT_CLKTCK`; there is no hard-coded `100 Hz` fallback. |
| Time and locale | `LC_ALL`/`LC_TIME` precedence, bounded locale month names, invocation-local `TZ`, recent/old start shapes, and POSIX elapsed/cumulative CPU shapes are covered. `LC_CTYPE` controls byte versus multibyte display-column accounting. |
| Width and output | Valid `COLUMNS` overrides the live output-terminal width. Every line is bounded in text columns, including wide and combining UTF-8 characters. Short writes and writer errors diagnose and return status 1. |
| Diagnostics/status | Syntax errors return 2; locale, provider, enumeration, and output failures return 1; success returns 0. Standard error is diagnostic-only. SIGTTIN retains its default job-control disposition. |

## Extensions and Residuals

- Long option aliases, `--help`, `--version`, and the extra `f`, `s`, `uid`,
  `gid`, `c`, `pri`, `addr`, `sz`, `wchan`, and `start` format names are Bashy
  extensions. They do not alter the Issue 7 short-option interface.
- Diagnostic strings are English. The invocation accepts `LC_MESSAGES` and
  `NLSPATH`, but this command ships no translated message catalog, so they
  cause no diagnostic-language change.
- Linux `SZ`, scheduling percentage, priority, flags, address, state, and wait
  channel meanings are implementation-defined, as the XSI standard permits.
- No live non-Linux conformance claim is made. Supporting another target
  requires an exact provider for selection identity and terminal/session data,
  not merely a process-name list.

## Review Verification

The review ran the focused `cmds/ps` suite on Linux and Darwin both normally
and with `POSIXLY_CORRECT=1`, plus the package race test, vet, formatting,
generated-ledger consistency, and Linux/Darwin/Windows cross-build checks.
The repository-wide manifest validator remains blocked by the pre-existing
`sh: partial state requires focused semantic evidence` ledger defect. These
checks support the retained `partial` status; they do not replace the missing
full Issue 7 harness run.
