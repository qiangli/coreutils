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
| mv | guonaihong, u-root | -f, -n, -v |
| rm | u-root | -r/-R, -f, -v, -i refused (interactive) |
| mkdir | u-root | -p, -m, -v |
| rmdir | guonaihong | -p, -v |
| touch | guonaihong, u-root | -a, -m, -c, -d, -r, -t |
| ln | u-root | -s, -f, -v |
| link / unlink | guonaihong, u-root | trivial pair |
| mktemp | u-root | -d, -p, -u, templates |
| truncate | u-root | -s (K/M/G suffixes), -c |
| chmod | guonaihong, u-root | octal + symbolic; **unix only** — clear error on Windows (no POSIX mode bits; mapping to read-only would change the documented meaning) |
| chown / chgrp | guonaihong | **unix only**, same rule |

Listing and filesystem info:

| Command | Sources | Notes |
|---|---|---|
| ls | aict, u-root | -l, -a, -A, -d, -R, -r, -t, -S, -1, -h, -i; C-locale byte-order sort, no color |
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

Checksums and encoding:

| Command | Sources | Notes |
|---|---|---|
| base64 / base32 | guonaihong, u-root | -d, -i, -w |
| md5sum | guonaihong, u-root | -c, -b, --tag |
| sha1/224/256/384/512sum | guonaihong, u-root (shasum) | one shared engine |

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
| expr | arithmetic + string ops |
| nl | -b, -n, -s, -w subset |
| fold | -w, -b, -s |
| expand / unexpand | -t; -a (unexpand) |
| od | -A, -t, -N, -j subset |
| cksum | POSIX CRC default |
| b2sum | needs x/crypto (dep-budget review) |
| basenc | covers base64/32 variants beyond the Phase A pair |
| dd | simplified: if/of/bs/count/skip/seek/status=none/conv subset |
| install | -d, -m, -D subset |
| csplit | split's sibling |
| numfmt | --to/--from common units |
| nproc | --all, --ignore |
| arch | trivial `uname -m` alias |
| tail -f | follow mode for the Phase A tail (polling, cross-platform) |
| coreutils | the multicall binary itself (`cmd/coreutils`) |

## Phase C — remaining extensions (beyond coreutils)

sed; xargs (lands with in-process command execution via the sh
ExecHandler); ps (agent-useful, large cross-platform surface); file
(magic detection). grep/find/diff/tar/gzip already land in Phase A via
prior art.

## NO — not supported (clear error, by reason)

**Requires executing other programs** (violates no-shell-out; ↻ =
revisit when the sh ExecHandler can run commands in-process):

- timeout ↻, nohup ↻, env COMMAND ↻, time ↻, nice, stdbuf, chroot
- kill — already a builtin in the qiangli/sh fork; a standalone would race it

**Unix machinery with no cross-platform meaning:**

- mkfifo, mknod, stty, chcon, runcon, hostid
- who, users, pinky, groups, logname (whoami covers identity)

**Low agent value / legacy / dangerous:**

- ptx, factor, pr, fmt, dircolors, dir, vdir, sum, pathchk
- shred (lies on SSDs/COW filesystems), more/man (interactive pagers)

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
