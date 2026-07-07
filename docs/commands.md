# Command plan

The canonical supported / not-supported inventory, derived from the
[GNU coreutils manual](https://www.gnu.org/software/coreutils/manual/coreutils.txt)
command list plus the agent-critical extensions. Three rules frame
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
| ln | u-root | -s, -f, -v |
| link / unlink | guonaihong, u-root | trivial pair |
| mkfifo | fresh | -m octal; Unix native, clear unsupported error elsewhere |
| mknod | fresh | NAME TYPE [MAJOR MINOR], -m octal; Unix native, clear unsupported error elsewhere |
| mktemp | u-root | -d, -p, -u, templates |
| truncate | u-root | -s (K/M/G suffixes), -c |
| dd | fresh | if/of/bs/ibs/obs/count/skip/seek/status=none\|noxfer/conv=notrunc; POSIX seek= semantics (preserves skipped blocks, truncates at seek offset); obs re-blocking (bs= writes as read, per GNU); trailer is a plain "N bytes copied" — no timing/throughput (deterministic-output deviation) |
| shred | fresh | -n, -z, -u, -f, -v; warns by documentation caveat, regular files only; -u truncates+unlinks without GNU wipesync's rename-to-shorter-names pass (documented deviation) |
| chmod | guonaihong, u-root | octal + symbolic; **unix only** — clear error on Windows (no POSIX mode bits; mapping to read-only would change the documented meaning) |
| chown / chgrp | guonaihong | **unix only**, same rule |
| chcon | fresh | CONTEXT FILE... via Linux `security.selinux` xattr; clear unsupported error elsewhere |

Listing and filesystem info:

| Command | Sources | Notes |
|---|---|---|
| ls | aict, u-root | -l, -a, -A, -d, -R, -r, -t, -S, -1, -h, -i; C-locale byte-order sort, no color |
| dir / vdir | ls variant | delegate to ls / ls -l, answering --help/--version as themselves; GNU's -C/-b column/escape modes are not in ls, so output is ls's deterministic one-per-line (documented deviation) |
| dircolors | fresh | Bourne/C-shell LS_COLORS output in database order; GNU TERM/COLORTERM gating (pre-TERM entries global); unrecognized keywords and malformed lines are errors; built-in database emitted independent of $TERM (deterministic deviation) |
| stat | aict | default + -c format subset |
| du | aict, u-root | -s, -h, -a, -c, -d |
| df | aict, u-root | -h, -k; platform probes behind build tags |
| pwd | aict, guonaihong, u-root | -L, -P |
| realpath | aict, guonaihong, u-root | -e, -m, -s, --relative-to |
| readlink | u-root | -f, -e, -m, -n |
| basename | all three | exemplar |
| dirname | all three | -z |
| sync | u-root | fsync named files; bare sync unix-only |
| which | u-root | PATH search (reports real binaries; the in-shell story is the ExecHandler's) |

Text — reading and slicing:

| Command | Sources | Notes |
|---|---|---|
| cat | all three | -A, -b, -e, -E, -n, -s, -t, -T, -u, -v |
| head | aict, guonaihong, u-root | -n (incl. -NUM), -c, -q, -v |
| tail | aict, guonaihong, u-root | -n (incl. +N), -c, -q, -v; **-f in Phase B** |
| wc | aict, guonaihong, u-root | -l, -w, -c, -m, -L |
| tac | guonaihong | default + -s |
| split | guonaihong | -l, -b, -n, -d, -a |
| cmp | u-root | -l, -s (diffutils, but prior art covers it) |
| strings | u-root | -n, -t |
| hexdump | u-root | -C subset (od lands in Phase B) |
| csplit | fresh | line numbers, /REGEXP/, %REGEXP%, -f, -n, -s, -k |
| nl | fresh | -b a/t/n, -n ln/rn/rz, -s, -w |
| od | fresh | default octal words; -A d/o/x/n, -t x1/o1/o2/c/d1, -N, -j |
| pr | fresh | sequential non-interactive pagination; -l, -w, -t |
| more | fresh | non-interactive pager fallback; stdin/files passthrough |

Text — transform and combine:

| Command | Sources | Notes |
|---|---|---|
| sort | aict, guonaihong, u-root | -r, -n, -u, -f, -b, -k, -t, -o, -s, -c, -h; byte order |
| uniq | aict, guonaihong, u-root | -c, -d, -u, -i, -f, -s, -w |
| cut | aict, guonaihong | -b, -c, -f, -d, -s, --complement |
| tr | aict, guonaihong, u-root | SET1/SET2, -d, -s, -c, classes |
| comm | u-root | -1, -2, -3 |
| join | guonaihong | -1, -2, -t, -a, -v, -i subset |
| paste | guonaihong | -d, -s |
| tee | guonaihong, u-root | -a, -i |
| tsort | u-root | (in prior art, so it rides along) |
| shuf | guonaihong | -n, -e, -i; randomness is the upstream-documented exception to determinism |
| expand / unexpand | fresh | -t; -a for unexpand |
| fold | fresh | -w, -b, -s |
| fmt | fresh | practical paragraph wrapping; -w, -s |
| numfmt | fresh | --from=none/auto/si/iec/iec-i; --to=none/si/iec/iec-i; --format |
| ptx | fresh | deterministic permuted index; -f, -i, -o |

Environment, system, misc:

| Command | Sources | Notes |
|---|---|---|
| echo | guonaihong, u-root | -n, -e, -E (the sh-shell builtin wins in-shell) |
| yes | guonaihong, u-root | |
| true / false | guonaihong, u-root | exemplars — already in tree |
| env | aict, guonaihong | print, -i, -u, NAME=VALUE; **running COMMAND is NO for now** (process execution; revisits with the sh ExecHandler) |
| printenv | u-root | |
| date | u-root | strftime +FORMAT, -u, -d subset, -r; C locale |
| sleep | guonaihong, u-root | suffixed durations, multiple args |
| seq | guonaihong, u-root | -s, -w, -f |
| uname | guonaihong, u-root | -s, -n, -r, -m, -o, -a |
| whoami | guonaihong | |
| hostname | u-root | print only |
| tty | u-root | |
| id | u-root | unix semantics; Windows best-effort per platform note |
| uptime | u-root | platform probes |
| arch | uutils parity | prints machine hardware name |
| chroot | uutils parity | native chroot on Unix; --userspec, --groups, --skip-chdir; requires privileges and mutates process root as the utility semantics require |
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
| runcon | uutils parity | Linux SELinux procfs contexts; clear unsupported error elsewhere |
| stdbuf | uutils parity | -i/-o/-e parsing and COMMAND environment; depends on target program/libstdbuf support |
| stty | uutils parity | terminal status, size, selected modes/settings; platform terminal support required |
| users | uutils parity | utmp-backed logged-in user list; optional FILE |
| who | uutils parity | utmp-backed listing with main flags, count and heading modes; optional FILE / ARG1 ARG2 |

Checksums and encoding:

| Command | Sources | Notes |
|---|---|---|
| base64 / base32 | guonaihong, u-root | -d, -i, -w |
| basenc | written fresh | --base64, --base64url, --base32, --base32hex, --base16; -d, -i, -w |
| b2sum | written fresh | BLAKE2b-512 via shared checksum engine |
| cksum | written fresh | POSIX CRC default |
| md5sum | guonaihong, u-root | -c, -b, --tag |
| sha1/224/256/384/512sum | guonaihong, u-root (shasum) | one shared engine |
| sum | written fresh | BSD default/-r and System V -s |

Extensions (beyond coreutils, prior art in tree):

| Command | Sources | Notes |
|---|---|---|
| grep | aict, u-root | -r, -i, -l, -n, -v, -E, -F, -c, -q, -m, --include/--exclude |
| find | aict, u-root | -name, -type, -maxdepth, -mtime, -size, -path, -prune, -print0; **-exec is NO** |
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
| test / [ | standalone (the sh interp builtin covers in-shell use) |
| tail -f | follow mode for the Phase A tail (polling, cross-platform) |
| coreutils | the multicall binary itself (`cmd/coreutils`) |

## Phase C — extensions (beyond coreutils)

Shipped: sed; xargs (command-wrapper tier, see the NO list — spawns
COMMAND directly; GNU subset incl. -I, -P, -0, -d); awk (goawk); plus
the agent-oriented extras not tracked by this GNU-manual inventory
(watch, tree, cal, time, timeout, at/atq/atrm/batch/crontab, browser,
fetch, clip, tokens, duration, tz, ntp — see `cmds/all/all.go` for the
authoritative shipped set). grep/find/diff/jq/tar/gzip landed in
Phase A via prior art.

Remaining: ps (agent-useful, large cross-platform surface); file
(magic detection).

## NO — not supported (clear error, by reason)

**Requires executing other programs.** The no-shell-out rule bars a
tool from spawning programs to *implement its own behavior* (cat never
execs /bin/cat). **Command wrappers are the documented exception**:
tools whose upstream-documented purpose IS running the COMMAND operand
(timeout, time, watch, xargs — all shipped) spawn that command
directly, exactly as the GNU binary does — that is the upstream
semantics, not an implementation shortcut. Still NO (↻ = revisit):

- env COMMAND ↻
- kill — already a builtin in the qiangli/sh fork; a standalone would race it

**Unix machinery with no cross-platform meaning:**

- none currently from the coreutils shell-utils gap; platform-specific commands now return real native behavior where supported and clear unsupported errors otherwise

**Low agent value / legacy / dangerous:**

- man (interactive pager)

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
