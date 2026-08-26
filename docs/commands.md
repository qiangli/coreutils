# Command plan

The canonical supported / not-supported inventory, derived from the GNU
Coreutils command list plus POSIX, Unix, and agent-critical extensions. The
[command behavior reference policy](reference-policy.md) is authoritative:
GNU Coreutils 9.11 applies only to commands that GNU Coreutils ships; POSIX
certification behavior and non-Coreutils command families use their specified
references. Three rules frame
every entry (see CLAUDE.md): implemented commands follow the official
documentation exactly (flags/options/arguments keep their upstream
meaning — the only other state is a clear "not supported" error),
nothing ever shells out, and behavior is identical on
linux/macos/windows unless a platform note says otherwise.

**Phasing rule (2026-06):** Phase A is the union of commands that have
a Go implementation in `priorart/` (aict, guonaihong/coreutils,
u-root) and don't hit a NO-reason below — adaptation beats
reinvention, so prior-art coverage is what sequences the work. Phase B
is everything else we want that must be written fresh. Conformance is
still judged against official docs, never against the prior art.

## Phase A — adapted from Go prior art (SHIPPED 2026-06)

File operations:

| Command | Sources | Notes |
|---|---|---|
| cp | u-root | -r/-R, -p, -f, -n, -v |
| install | fresh | -d, -D, -m, -v, -t, -T; ownership flags refused |
| mv | guonaihong, u-root | -f, -n, -v |
| rm | u-root | -r/-R, -f, -v, -i refused (interactive) |
| mkdir | u-root | -p, -m, -v |
| rmdir | guonaihong | -p, -v |
| touch | guonaihong, u-root | -a, -m, -c, -d, -r, -t |
| ln | u-root | -s, -f, -v; uutils-parity additions: -t/--target-directory, -T/--no-target-directory, -n/--no-dereference, -r/--relative |
| link / unlink | guonaihong, u-root | trivial pair |
| mkfifo | fresh | -m octal; Unix native, clear unsupported error elsewhere |
| mknod | fresh | NAME TYPE [MAJOR MINOR], -m octal; Unix native, clear unsupported error elsewhere |
| mktemp | u-root | -d, -p, -u, templates |
| truncate | u-root | -s (K/M/G suffixes), -c |
| dd | fresh; XSI conv=ascii/ebcdic/ibm tables adapted from FreeBSD `bin/dd/conv_tab.c` (BSD-3-Clause, see THIRD_PARTY_LICENSES.md) | if/of/bs/ibs/obs/count/skip/seek/status=none\|noxfer/conv=notrunc,noerror,sync,block,unblock,lcase,ucase,swab,ascii,ebcdic,ibm; POSIX seek= semantics (preserves skipped blocks, truncates at seek offset); output is re-blocked when a data conversion is active (bs= otherwise writes as read); GNU-compatible single-byte case mapping and odd-byte swab handling; conv=ascii/ebcdic/ibm are mutually exclusive, each requires cbs=, and each implies unblock (ascii) or block (ebcdic/ibm) per POSIX XCU dd; trailer is a plain "N bytes copied" — no timing/throughput (deterministic-output deviation) |
| shred | fresh | -n, -z, -u, -f, -v; warns by documentation caveat, regular files only; -u truncates+unlinks without GNU wipesync's rename-to-shorter-names pass (documented deviation) |
| chmod | guonaihong, u-root | octal + symbolic; **unix only** — clear error on Windows (no POSIX mode bits; mapping to read-only would change the documented meaning) |
| chown / chgrp | guonaihong | **unix only**, same rule |
| chcon | fresh | CONTEXT FILE... via Linux `security.selinux` xattr; clear unsupported error elsewhere |

Listing and filesystem info:

| Command | Sources | Notes |
|---|---|---|
| ls | aict, u-root | Broad uutils option surface: -l, -a, -A, -d, -R, -r, -t, -S, -1, -h, -i plus display/sort/quoting/filtering/dereference additions from the 2026-07-07 parity sprint; deterministic one-entry-per-line output for column/row width modes; no color emission |
| dir / vdir | ls variant | delegate to ls / ls -l, answering --help/--version as themselves; GNU's -C/-b column/escape modes are not in ls, so output is ls's deterministic one-per-line (documented deviation) |
| dircolors | fresh | Bourne/C-shell LS_COLORS output in database order; GNU TERM/COLORTERM gating (pre-TERM entries global); unrecognized keywords and malformed lines are errors; built-in database emitted independent of $TERM (deterministic deviation) |
| stat | aict | default + -c format subset |
| du | aict, u-root | -s, -h, -a, -c, -d |
| df | aict, u-root | POSIX Issue 7 XSI interface in progress: 512-byte units by default, -k for 1024, -P portable format, default/-t free-file-slot fields, and -t total-allocated-space records; signed free space and APFS volume allocation supported; LC_MESSAGES catalogs remain a documented residual, so the interface state remains partial; GNU aggregate totals remain available only as --total |
| pwd | aict, guonaihong, u-root | -L, -P |
| realpath | aict, guonaihong, u-root | -e, -m, -s, --relative-to; uutils-parity additions: -E/--canonicalize, -L/--logical, -P/--physical, -q/--quiet, -z/--zero, --relative-base |
| readlink | u-root | -f, -e, -m, -n; uutils-parity additions: -q/--quiet, -s/--silent, -v/--verbose, -z/--zero |
| basename | all three | exemplar |
| dirname | all three | -z |
| sync | u-root | fsync named files; bare sync unix-only |
| which | u-root | PATH search (reports real binaries; the in-shell story is the ExecHandler's) |

Text — reading and slicing:

| Command | Sources | Notes |
|---|---|---|
| cat | all three | -A, -b, -e, -E, -n, -s, -t, -T, -u, -v |
| head | aict, guonaihong, u-root | -n (incl. -NUM), -c, -q, -v; uutils-parity addition: -z/--zero-terminated |
| tail | aict, guonaihong, u-root | -n (incl. +N), -c, -q, -v, -z/--zero-terminated; **-f in Phase B** |
| wc | aict, guonaihong, u-root | -l, -w, -c, -m, -L; uutils-parity additions: --files0-from, --total=auto/always/only/never |
| tac | guonaihong | default + -s |
| split | guonaihong | -l, -b, -n, -d, -a |
| cmp | u-root | -l, -s (diffutils, but prior art covers it) |
| strings | u-root | -n, -t |
| hexdump | u-root | -C subset (od lands in Phase B) |
| csplit | fresh | line numbers (repeats advance by N; {*} to EOF), /BRE/[+-N], %BRE%[+-N] via pkg/bre, {N}/{*}, -f, -n, -b, -s, -k, -z, --suppress-matched |
| nl | fresh | -b/-h/-f a/t/n/pBRE (pkg/bre), GNU section delimiters (replaced by empty lines), one document across files, unnumbered lines padded width+len(sep), -d, -v/-i (negatives ok), -l, -p, -n ln/rn/rz, -s, -w |
| od | fresh | default octal words + GNU * duplicate elision (-v disables); -A d/o/x/n, POSIX concatenated -t types with C ABI sizes, -a/-b/-c/-d/-o/-s/-x, -N, -j (errors past EOF), -S (NUL-terminated), -w, --endian, traditional +offset (octal, ./0x/b); GNU traditional LABEL and alternate operand forms are deferred outside the POSIX-focused Sprint 79 scope |
| pr | fresh | GNU page model (66-line pages, 5-line header/trailer, bottom fill, -l≤10 implies -t, FF page breaks, +FIRST[:LAST]); -l, -w/-W, -t/-T, -h, -o, -d, -n; single-column never truncated; multi-column/-m refused loudly; stdin header uses wall clock (documented deviation) |
| more | fresh | terminal pager over an injectable controlling-terminal channel: geometry -n/$LINES/tty/24 by $COLUMNS/tty/80, screenful rows-1, backspace/CR/tab column folding, --More--/(Next file)/(END) prompts, -e, -c, -u, -s, POSIX -p COMMAND at each new file's first screen; this slice implements <space>/q only and refuses every other more(1) command as deferred; -i/-t refused; fails closed to the non-interactive passthrough (stdin/files, only -s in effect per POSIX) when stdout is not a terminal or the channel cannot be opened; -P literal pattern ("Pattern not found" fallback) |

Text — transform and combine:

| Command | Sources | Notes |
|---|---|---|
| sort | aict, guonaihong, u-root | -r, -n, -u, -f, -b, -k, -t, -o, -s, -c, -h; C/POSIX byte order, narrow LC_COLLATE text ordering, and narrow LC_NUMERIC `-n` radix/thousands support; `-g`/`-h` remain byte-defined |
| uniq | aict, guonaihong, u-root | -c, -d, -u, -i, -f, -s, -w; uutils-parity additions: -z, -D/--all-repeated[=METHOD], --group[=METHOD] |
| cut | aict, guonaihong | -b, -c, -f, -d, -s, --complement |
| tr | aict, guonaihong, u-root | SET1/SET2, -d, -s, -c, classes |
| comm | u-root | -1, -2, -3 |
| join | guonaihong | -1, -2, -t, -a, -v, -i subset |
| paste | guonaihong | -d, -s; uutils-parity addition: -z/--zero-terminated |
| tee | guonaihong, u-root | -a, -i; uutils-parity additions: -p/--ignore-pipe-errors, --output-error[=MODE] |
| tsort | u-root | (in prior art, so it rides along) |
| shuf | guonaihong | -n, -e, -i; randomness is the upstream-documented exception to determinism |
| expand / unexpand | fresh | -t lists incl. GNU +N//N prefixes, repeats accumulate, blank-separated; -i (expand); -a/--first-only (unexpand) with GNU's 2+-blank rule, beyond-last-stop blanks kept, blank+tab runs merged, backspace column tracking; uutils-parity addition: -U/--no-utf8 byte-column mode |
| fold | fresh | -w/-WIDTH, -b, -c, -s; GNU screen-column counting (tab→next stop of 8, BS decrements, CR resets), -s keeps the break blank (never deletes bytes) |
| fmt | fresh | GNU surface (-c, -t, -s, -u, -w/-WIDTH, -g, -p): paragraph indents preserved, different indents never join, goal per GNU (93% of width; -g without -w caps at 75 like GNU source); greedy filling + single-space normalization are documented deviations |
| numfmt | fresh | --from=none/auto/si/iec/iec-i; --to=none/si/iec/iec-i with GNU human rounding (&lt;10 one decimal, else integer); field 1 default + implicit width padding; validated --format (%f family; ' and --grouping are C-locale no-ops); --round, --invalid, --header, -z, -d, --field |
| ptx | fresh | GNU dumb-terminal output (width 72, gap 3, / truncation marks), roff via -O, -t=width 100, -f folds to upper, case-sensitive -i/-o unless -f, -A file:line refs, -G/-b/-S/-W; line-scoped contexts + Go-regexp -S/-W are documented deviations; -T refused |

Environment, system, misc:

| Command | Sources | Notes |
|---|---|---|
| echo | guonaihong, u-root | -n, -e, -E (the sh-shell builtin wins in-shell) |
| yes | guonaihong, u-root | |
| true / false | guonaihong, u-root | exemplars — already in tree |
| env | aict, guonaihong | print, -i, -u, NAME=VALUE, `-`, `--`; runs COMMAND (command-wrapper tier — see the NO-list preamble): direct exec, no shell, POSIX PATH search (a zero-length prefix, including a wholly empty PATH, means the working directory), 126/127 lookup status, child killed with the invocation context |
| printenv | u-root | |
| date | u-root | strftime +FORMAT, -u, -d subset, -r; C locale |
| sleep | guonaihong, u-root | suffixed durations, multiple args |
| seq | guonaihong, u-root | -s, -w, -f |
| uname | guonaihong, u-root | -s, -n, -r, -m, -o, -a; uutils-parity addition: -v/--kernel-version |
| whoami | guonaihong | |
| hostname | u-root | print mode plus uutils query flags -d/--domain, -f/--fqdn, -i/--ip-address, -s/--short; setting HOSTNAME still refused |
| tty | u-root | uutils-parity additions: -s/--silent, --quiet |
| id | u-root | unix semantics; Windows best-effort per platform note |
| uptime | u-root | platform probes; uutils-parity additions: -p/--pretty, -s/--since |
| arch | uutils parity | prints machine hardware name |
| chroot | release-withheld | implementation exists, but is excluded from `cmds/all` until bounded privileged integration coverage proves root change, credential handling, command status, and cleanup |
| expr | uutils parity | arithmetic, comparison, boolean, regex match, length/index/substr/match/quote |
| factor | uutils parity | decimal integers, stdin splitting, -h/--exponents |
| groups | uutils parity | current user or named users via OS account database |
| hostid | uutils parity | 8-hex host identifier; pure-Go fallback when libc gethostid is unavailable |
| logname | uutils parity | login/current account name |
| nice | uutils parity | prints current niceness or runs COMMAND with -n/--adjustment; wrapper exit codes 126/127 |
| nohup | uutils parity | runs COMMAND with output appended to nohup.out when possible; wrapper exit codes 126/127 |
| nproc | uutils parity | --all, --ignore, OMP_NUM_THREADS/OMP_THREAD_LIMIT, Linux cgroup quota best effort |
| pathchk | uutils parity | -p, -P, --portability |
| pinky | uutils parity | utmp-backed short listing and long user format; empty output when no records are available |
| printf | written fresh | POSIX FORMAT reuse (stops after one pass if nothing was consumed) with excess/missing-operand defaults; %s %b %c %d %i %o %u %x %X plus %e/%E/%f/%F/%g/%G/%a/%A; width/precision incl. `*` argument forms (negative width left-justifies, negative precision is omitted); backslash escapes incl. `\NNN` octal and `\c` output-termination (in FORMAT and inside %b, whose own octal form is `\0NNN`); POSIX leading-quote numeric character constants; invalid numbers/conversions diagnosed with nonzero status, `--` ends option scanning |
| runcon | release-withheld | implementation exists, but is excluded from `cmds/all` until bounded SELinux integration coverage proves label transition, command status, restoration, and unsupported-platform behavior |
| stdbuf | uutils parity | -i/-o/-e parsing and COMMAND environment; depends on target program/libstdbuf support |
| stty | uutils parity | terminal status, size, selected modes/settings; platform terminal support required |
| tabs | written fresh | POSIX preset formats (-a, -a2, -c, -c2, -c3, -f, -p, -s, -u), the repetitive -n spec (-0 clears; multi-digit accepted as every historic implementation does), explicit strictly-ascending lists with `+` increments, `+m[n]` / `+[n]` left margin, `-T`; renders through the shared pure-Go terminfo reader (cmds/internal/terminfo). Deviations: the POSIX <blank> separator IS accepted (BSD tabs rejects it), and a margin the terminal cannot set is reported AND carried into a nonzero exit rather than warned about at exit 0 |
| users | uutils parity | utmp-backed logged-in user list; optional FILE |
| who | uutils parity | native Linux/Darwin session databases; POSIX selectors, `-s` suppression, count/heading modes, optional FILE / `am i`; full POSIX TZ rules via the shared provider; LC_TIME uses the bounded C/POSIX and de_DE UTF-8/ISO-8859-1 corpus and fails closed by locale name when other time data is requested |

Checksums and encoding:

| Command | Sources | Notes |
|---|---|---|
| base64 / base32 | guonaihong, u-root | -d, -i, -w |
| basenc | written fresh | --base64/url, --base32/hex, --base16, --base2msbf/lsbf, --z85, --base58; -d (GNU ≥9.5 semantics: auto-pads unpadded input, rejects non-zero padding bits), -i, -w; last encoding flag wins |
| b2sum | written fresh | BLAKE2b via shared checksum engine; -l (0 = default 512), --tag with BLAKE2b-&lt;len&gt; labels, -c auto-detects digest length (untagged by hex count, tagged by suffix), --warn/--status/--quiet/--strict gated on -c |
| cksum | written fresh | POSIX CRC default; -a GNU set (bsd/sysv/crc/crc32b decimal + md5/sha*/sha2/sha3+-l/blake2b/sm3, exact-match names) with tagged-only -c auto-detect per GNU; --raw incl. crc family; blake3/shake/sha3-NNN accepted as documented extensions |
| md5sum | guonaihong, u-root | -c, -b, --tag |
| sha1/224/256/384/512sum | guonaihong, u-root (shasum) | one shared engine |
| sum | written fresh | BSD default/-r and System V -s (last flag wins, `-` prints its name; -r is short-only as in GNU) |

Extensions (beyond coreutils, prior art in tree):

| Command | Sources | Notes |
|---|---|---|
| grep | aict, u-root | -r, -i, -l, -n, -v, -E, -F, -c, -q, -m, -A/-B/-C context, ordered --include/--exclude |
| sed | Go.Sed engine (MIT), GNU compatibility adaptations | GNU BRE/ERE scripts, addr,+N ranges, -i[SUFFIX] temporary-file in-place editing; POSIX single-byte locale classes, equivalence classes, collating elements, and LC_COLLATE-ordered ranges for the bounded de_DE ISO-8859-1 provider (multi-character collating elements fail closed) |
| find | aict, u-root | POSIX stream closed 2026-08: -H/-L/-P (and `--` closing them), -name/-iname/-path (matched in the C locale: byte-wise, ASCII classes, no LC_*/LANG variance), -type, -atime/-ctime/-mtime, -newer, -size, -empty, -perm (octal + symbolic, -/ prefixes), -user/-group/-nouser/-nogroup, -links, -xdev, -depth, -prune, -maxdepth/-mindepth, -print/-print0, **-exec ; and -exec {} +, interactive -ok** (command-wrapper tier: running the utility is find's specified behavior; argv built directly, never through a shell). Still NO: -execdir/-okdir/-delete |
| diff | aict | -u, -r, -q (unified output) |
| jq | gojq | pure-Go JSON filters; initial flags -c, -e, -n, -r |
| tar | u-root | -c, -x, -t, -z, -f (archive/tar + compress/gzip) |
| gzip / gunzip | u-root | -d, -k, -c, -1..-9 |
| git | — | done (`git/` package) |

## Phase B — coreutils completion (written fresh)

The exact complement: every program in the GNU coreutils manual that is
not in Phase A and not on the NO list. Nothing in the manual is
unaccounted for — each program is in exactly one of Phase A, Phase B,
or NO.

| Command | Notes |
|---|---|
| printf | %s %d %x %o %c %b %% escapes, width/precision |
| test / [ | **shipped** (`cmds/test`) — standalone, one implementation under both names, registered separately so each keeps its own option rule. Every POSIX primary: unary file tests (-e/-a, -f, -d, -s, -b, -c, -p, -S, -h/-L, -u, -g, -k, -r, -w, -x, -O, -G, -N, -t), string (-n, -z, =, ==, !=, <, >), integer (-eq/-ne/-lt/-le/-gt/-ge, arbitrary precision), file comparison (-ef, -nt, -ot), plus `!`, `-a`/`-o`, and `(`…`)`. POSIX 0–4-operand dispatch exactly as specified, with the documented recursive-descent grammar beyond it; neither operator short-circuits, so a malformed branch is always reported. `[` requires the closing `]` (a missing one is `[: missing ']'`, exit 2). Options follow upstream: `test` has none at all (`test --help` is a true string test), while `[ --help`/`[ --version` work only as the single argument, matched literally with no abbreviation — so this tool bypasses `tool.Parse`. Exit 0 true / 1 false / 2 syntax-or-usage. The sh interp builtin still covers in-shell use. |
| tail -f | follow mode for the Phase A tail (polling, cross-platform) |
| coreutils | the multicall binary itself (`cmd/coreutils`) |

## Phase C — extensions (beyond coreutils)

Shipped: sed; xargs (command-wrapper tier, see the NO list — spawns
COMMAND directly; GNU subset incl. -I, -P, -0, -d); awk (goawk);
file (portable built-in signature set); iconv (Go IANA encoding registry);
uuencode/uudecode (POSIX classic and `-m` base64 framing, pathname, and access
bit behavior; locale codeset and message-catalog effects remain unverified); plus
the agent-oriented extras not tracked by this GNU-manual inventory
(watch, tree, cal, time, timeout, at/atq/atrm/batch/crontab, browser,
fetch, clip, tokens, duration, tz, ntp, why — see `cmds/all/all.go` for the
authoritative shipped set). grep/find/diff/jq/tar/gzip landed in
Phase A via prior art.

Remaining: richer optional file magic/MIME coverage and broader `ps` format
coverage beyond the portable POSIX foundation.

### Portable userland and certification-provider policy

Bashy's product goal is a useful, tested, pure-Go userland with coherent
semantics on Linux, macOS, and Windows. It is **not** a promise to reproduce
every historical POSIX utility in Go. A command belongs in the advertised
multicall inventory only when its supported behavior is valuable on those
platforms and its package and platform coverage satisfy the release gate.

The formal POSIX Shell and Utilities profiles are a separate concern:

- **Profile B** is the Bashy shell with frozen GNU/system utility providers.
  It excludes this repository's Go applets so that it isolates shell behavior.
- **Profiles C/D** place the Bashy Go multicall provider first, with GNU Bash
  or Bashy respectively, and therefore measure this repository's applets.

Certification evidence applies only to the exact staged profile manifest.
Host fallback must be measured and recorded and never credited as a Bashy Go
implementation.

The generated [POSIX required-command coverage
map](posix-required-commands.md) remains the exact five-column A/B/C/D harness
contract: 116 configured names, with availability of 90 Go applets, 14
shell-only names, and 12 active pinned providers. Expanded interface claims live in a
separate [evidence ledger](posix-required-command-interfaces.md), with effective
Profile C/D ownership of 82 Go-selected, 22 shell-selected, and 12 provider
commands. The ledger is explicitly incomplete and non-normative; it exposes
missing, partial, implemented, and verified states rather than treating
placeholders as conformance evidence.

**A provider is not a Go applet, and the matrix counts it separately so it can
never be read as Go coverage.** The multicall owns the name (`make`, `bc`,
`m4`, `man`, `ctags`, `ar`, `nm`, `strip`, `ex`, `vi`, `lp`, `localedef`) and
dispatches to a copy of the upstream program built locally from a sha256-pinned
source tarball. Owning the name is precisely what stops a "Bashy-only" arm from
silently measuring the host's `$PATH`, which is what happened while these names
were unregistered. There is no fallback: an unprovisioned provider exits 127.
See [POSIX external providers](posix-external-providers.md).

`posix-gate` (`cmds/posixgate`) is the fail-closed gate over the whole
116-name inventory (availability 90/14/12, effective selection 82/22/12): it
proves the assembled runtime selects each name's intended owner — Go applet,
shell builtin/keyword/entry, or pinned provider — and rejects count drift on
either axis, ambiguous ownership, missing provider pins/provenance, host PATH
fallback, a staged executable or shell whose digest is not the approved build
recorded in the externally supplied build manifest (identity is mandatory and
profile-exact: GNU bash 5.3 with the stock -release flavor for Profile C,
GNU bash 5.3 with the Bashy-specific -bashy-<revision> marker for Profile D), a
provider cache the staged wrapper's own dispatch plan does not actually use, a
shell not in POSIX mode, and POSIXLY_CORRECT failing to reach child processes.
See [the POSIX owner gate](posix-owner-gate.md).

The provider allocation is maintained once, in the umbrella
[`posix-utility-provider-strategy.md`](../../docs/posix-utility-provider-strategy.md).
Provider commands are not automatically an implementation backlog.

Historical note: the 2026-08-05 staged snapshot contained 30 external-provider
names. `file`, `iconv`, `uudecode`, and `uuencode` subsequently became
registered Go applets. `ps` also landed but was not one of those 30 names.

The classes are prioritization guidance, not permanent prohibitions. Bashy may
also ship modern agent-oriented tools that solve current workflows better than
a legacy interface, under distinct names and documented semantics. Any newly
implemented POSIX/GNU name still must be registered, behaviorally and
cross-platform tested, listed in the generated applet matrix, and proven as the
actual staged provider before reports attribute it to Bashy.

#### Canonical multicall build (`cmd/coreutils`)

`cmd/coreutils` → `cmds/all` is the one shipped multicall inventory. There is
no reduced build with a different command set: consumers, documentation and
certification tooling all resolve the same registered Go implementations.
Whether a command is required by the POSIX certification tests is documentation
metadata in the generated applet matrix, not a build-time inclusion switch.

#### Portable-command implementation TODO

Priority is based on cross-platform agent value, implementation risk, and the
ability to provide honest semantics without delegating to a host executable.

- **P0 — initial implementations complete:** `file`, `iconv`, `uudecode`, and
  `uuencode`. Their useful documented subsets fail loudly for unsupported
  formats, encodings, and variants; continue hardening them with VSC deltas and
  cross-platform behavioral cases rather than silently broadening semantics.
- **P1 — continue scoped implementations:** broaden `ed` according to its
  [explicit continuation ledger](ed-posix-continuation.md), and investigate
  permissive Go prior art before scoping `patch`.
- **P1 — local implementation landed:** `mail`/`mailx` now provide a pure-Go,
  local-file mbox send/receive surface with no SMTP or network transport.
  Continue the interactive command/state lanes from
  the [mailx continuation ledger](mailx-continuation-ledger.md); the older
  `mailx-pax-make-agentic-plan.md` is historical, `pax` is shipped, and `make`
  remains a pinned external provider.
- **Release rule:** an implementation leaves this TODO only when its command
  package is tested, registered in `cmds/all`, classified in `pkg/atlas`,
  present in the generated applet matrix, and passes `scripts/crossvet.sh`.
- **Certification rule:** until that release rule is met and staged provider
  resolution is proven, the VSC profile continues to record the corresponding
  command as an external provider.

### at / batch / crontab — execution requires the schedule daemon

`at`, `batch`, and `crontab` submit/install jobs into the persistent
`pkg/schedule` JSON store. Their submission, listing, removal, and output
channels (the job diagnostic on stderr in the traditional
`Mon Jan _2 15:04:05 2006` format; crontab silent on success) are
POSIX-conformant. Firing due jobs, however, is the responsibility of the
schedule daemon (`schedule daemon` / `schedule tick` / `schedule run`) —
exactly as system `atd`/`crond` fire jobs submitted by system
`at`/`crontab`. In a standalone coreutils invocation the daemon is not
auto-started, so persisted jobs remain pending until the daemon runs.

`crontab` stores the submitted table byte-for-byte for `-l` and `-e`, while
the scheduler separately retains compiled commands and `%`-derived standard
input. Cron jobs use an absolute `/bin/sh` default, an invocation-independent
`HOME`/`LOGNAME`/`PATH`/`SHELL` environment, and umask 0022. A scheduler host
must supply output-mail delivery; without one, cron execution fails before the
job is claimed or run. POSIX cron installation and execution fail closed on
Windows because shell and umask semantics cannot be guaranteed there. POSIX
`at`/`batch` execution likewise fails closed on non-Unix targets that cannot
provide the saved umask, separate session and process group, and no-controlling-
terminal guarantees; those targets remain build-supported, not silently
POSIX-emulated.

TODO (execution semantics): unattended `at`/`batch`/`crontab` execution
requires either (a) auto-starting `schedule daemon` from the submit path
or (b) host cron-service integration. `batch` is represented as the POSIX
equivalent of `at -q b -m now`, including load gating and completion mail.

## NO — not supported (clear error, by reason)

**Requires executing other programs.** The no-shell-out rule bars a
tool from spawning programs to *implement its own behavior* (cat never
execs /bin/cat). **Command wrappers are the documented exception**:
tools whose upstream-documented purpose IS running the COMMAND operand
(env, timeout, time, watch, xargs, and find's -exec/-ok primaries — all
shipped) spawn that command directly, exactly as the GNU binary does —
that is the upstream semantics, not an implementation shortcut. Still
NO (↻ = revisit):

- none currently; `kill` is both a shipped standalone Go applet and a shell
  builtin, with builtin resolution taking precedence inside the shell

**Unix machinery with no cross-platform meaning:**

- none currently from the coreutils shell-utils gap; platform-specific commands now return real native behavior where supported and clear unsupported errors otherwise

**Low agent value / legacy / dangerous:**

- man (interactive pager)

**Terminal messaging and the durable `bashy mb` alternative:**

- wall, write, mesg, talk

  These write to a TERMINAL. The message is ephemeral, it reaches only a
  party who is logged in *right now*, and `write` to a logged-out user is an
  outright error. That is the one case an agent userland most needs to serve
  and these cannot: an agent is usually not "logged in" at the moment you
  need to tell it something.

  `bashy mb` is the durable replacement and a strict superset for this purpose — a
  durable append-only board with per-reader cursors, so a message to an agent
  that is *not running* waits and is delivered when it next looks, and
  `mb --all` still answers "what was I told, and when" afterwards.
  `mb post` is wall; `mb send <agent>` is write; `bus subscribe
  --interrupt-from` is mesg (it governs who may break into a running turn).
  Pure-Go `talk` keeps the traditional logged-in-user boundary: it writes an
  invitation to an eligible local terminal and exchanges encrypted,
  authenticated datagrams through ephemeral AF_UNIX sockets. It stores no
  transcript, rejects remote hosts, and never contacts talkd. `wall`, `write`,
  and `mesg` retain their existing terminal-oriented behavior.

  **POSIX/campaign status:** `write`, `mesg` and `talk` belong to the optional
  POSIX User Portability Utilities group, while `wall` is not POSIX. The
  configured VSC-PCTS2016 POSIX08 campaign nevertheless contains named test
  sets for `write`, `mesg`, and `talk`. `talk` is now a partial Go owner while
  `write` and `mesg` retain their existing Go implementations. Standard
  optionality does not permit silently omitting a configured test set.

**System administration (in u-root's tree, out of scope for an agent
userland — outpost/ycode own these concerns):**

- mount/umount, ip, ping, netcat, netstat, wget, scp, sshd, dhclient,
  insmod/rmmod/lsmod, losetup, blkid, gpt, dmesg, free, hwclock,
  strace, init/shutdown/poweroff, and the rest of u-root's
  boot/kernel tooling

The multicall binary and the sh ExecHandler give recognized-but-NO
names the git-verbs treatment: a clear error naming the command, the
reason, and the nearest supported alternative — never a silent
fallthrough.
