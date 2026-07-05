# perfbench fidelity matrix — empirical frequency head vs GNU

**Date:** 2026-07-05 · **Host:** Mac Studio (darwin/arm64) · **Harness:** `coreutils/cmds/perfbench conformance`.

**Reference (all built from source into one prefix):** GNU coreutils **9.11** ·
grep **3.11** · sed **4.9** · gawk **5.3.1** · findutils **4.10.0** · bash **5.3**.
Both arms `LC_ALL=C`.

## Command set — the *empirically-derived* frequency head
Chosen from a **first-hand** scan of the real terminal-agent benchmarks
(Terminal-Bench + TB-2.0 human solutions, InterCode; see
`scratchpad/benchmarks2/empirical-frequency.md`), NOT second-hand data. Key
correction: **`find` is a synthetic-NL2Bash artifact** (37% there, but ~0.5% /
rank ~27–31 in real Terminal-Bench tasks) — so the head is the coreutils
text/file core + the `grep/sed/awk` filters, and `find` is demoted.

Matrix: **~80 authored flag×input cases across 14 commands** (`cases.go`), each
diffed **byte-for-byte** (stdout + stderr + exit) against GNU.

## Result — 12 / 14 commands 100% byte-identical to GNU 9.11

| command | ref pkg | match | diff | loud-skip | conformance% |
|---|---|---|---|---|---|
| cat | coreutils 9.11 | 10 | **1** | 0 | 90.9 |
| wc | coreutils | 8 | 0 | 0 | 100 |
| head | coreutils | 6 | 0 | 0 | 100 |
| tail | coreutils | 4 | 0 | 0 | 100 |
| sort | coreutils | 10 | 0 | 0 | 100 |
| uniq | coreutils | 4 | 0 | 0 | 100 |
| cut | coreutils | 7 | 0 | 0 | 100 |
| tr | coreutils | 5 | 0 | 0 | 100 |
| grep | grep 3.11 | 9 | 0 | 1 | 100 |
| sed | sed 4.9 | 7 | 0 | 0 | 100 |
| awk | gawk 5.3.1 | 5 | 0 | 0 | 100 |
| rm | coreutils | 1 | 0 | 0 | 100 |
| cp | coreutils | 1 | 0 | 0 | 100 |
| ls | coreutils | 0 | **1** | 0 | 0.0 (single error-case only) |

## The one real bug found — **lowercase OS error strings** (cross-cutting)

Two DIFFs, one root cause:

```
cat corpus/conf/nope.txt
  GNU  : cat: corpus/conf/nope.txt: No such file or directory
  bashy: cat: corpus/conf/nope.txt: no such file or directory
ls corpus/conf/nope.txt
  GNU  : ls: cannot access 'corpus/conf/nope.txt': No such file or directory
  bashy: ls: cannot access 'corpus/conf/nope.txt': no such file or directory
```

**Root cause:** `cmds/cat/cat.go:277` (`sysErr`) and `cmds/ls/ls.go:472`
(`errMsg`) unwrap `*fs.PathError` to `pe.Err`, whose `.Error()` is Go's
`syscall.Errno` string — **lowercase** by Go convention (`"no such file or
directory"`). GNU uses libc `strerror()`, which **capitalizes** the first letter
(`"No such file or directory"`, `"Permission denied"`, `"Is a directory"`, …).

**Scope:** this is NOT cat/ls-specific — it hits **every command's error path**
that renders an `errno` (ENOENT / EACCES / EISDIR / ENOTDIR / EEXIST …). The
matrix only tested cat/ls error cases; the bug is systemic.

**Recommended fix (one place, adopt everywhere):** add a shared
`tool.SysErrString(err) string` that unwraps `*fs.PathError`/`*os.PathError` and
**capitalizes the first rune** of the errno message (which reproduces glibc/BSD
`strerror` for the common errnos), then route every command's error formatting
through it (replace the per-command `sysErr`/`errMsg`/`pathErr` helpers). Re-run
this matrix + `make test-bash` (86/86) to verify no regression. This is a
high-leverage fidelity fix — one helper closes error-wording divergences across
the whole userland.

## Coverage gap (contract-correct, not a bug)
`grep -o` is unimplemented — bashy loudly declines (`unknown shorthand flag: 'o'`
+ "not every GNU flag is implemented"), correctly classified `loud-skip`. `grep
-o` is common in agent tasks; worth adding as a real feature (separate from the
fidelity fix).

## Harness fixes made this run (so the matrix isolates real bugs)
1. **Failed-arm detection** — a missing GNU reference binary now shows
   `FAIL(exit N)` instead of a 127-exit masquerading as a fast time (this bug
   made grep look 157× slower in the first perf run).
2. **GNU `argv[0]` = bare name** — GNU echoes its invoked path in error
   prefixes; execing by absolute path made every error case false-DIFF on the
   program-name prefix (`/…/sort:` vs `sort:`). Fixed by setting `cmd.Args[0]`.
3. **loud-skip wording** — recognizes bashy's contract loud-fail
   (`"not every GNU flag is implemented"`, `"unknown shorthand flag"`) so an
   honest "flag unsupported" is a `loud-skip`, not a `diff`.
4. **DIFF details** — the runner now prints the actual `gnu=%q bashy=%q` bytes
   per divergence.

## FIX LANDED (2026-07-05) — all 14 commands now 100%

The `tool.SysErrString`/`tool.SysErr` helper (capitalizes the errno like glibc
`strerror`, unwrapping `*PathError`/`*LinkError`/`*SyscallError`) was added to
`coreutils/tool/syserr.go` and adopted by delegating all 26 local error helpers
(`sysErr`/`errMsg`/`pathErr`/…) to it, plus `ln.reason` and `diff.errText`.
(`cp`/`mv` already capitalized; `gzip` already hardcoded the capitalized form.)

**Re-run: every head command 100% byte-identical to GNU** — `cat` 11/11, `ls`
1/1 (both previously the only failures); `grep -o` remains a `loud-skip`
(unimplemented flag, contract-correct — a real coverage gap to fill later, not a
fidelity bug).

**Regression gates — all green (the change is coreutils-only; `sh` untouched):**
- coreutils `go test ./cmds/... ./tool/`: **0 failures** (realpath test updated to the GNU-correct capitalized wording).
- bash-5.3 official compat `make test-bash-parallel`: **86 / 86** (bin/bash excludes coreutils by design — structurally unaffected, verified).
- yash POSIX conformance: **96% (1763/1826), 0 bashy-specific failures** — bashy at parity with GNU bash 5.3; no POSIX regression.

## COMPLETE matrix — all 62 present coreutils commands (2026-07-05)

Extended the matrix from the ~14-command head to **every present coreutils
command** (~200 cases), with determinism-aware design: byte-compare for
filters/checksums/encoders/string-ops; error-case (stderr+exit) for fs-mutators;
fixed-seed/fixed-input for `date`(@epoch)/`shuf`(-i 1-1); and **deliberately
excluded** the non-deterministic set (`yes` [infinite], `df`/`uptime` [fluctuating
state], `mktemp` [random], `shuf` permutations, `split` [side-effect files]).

**Result: 62 / 62 commands 100% byte-identical to GNU 9.11** — verified on BOTH
native darwin/arm64 AND a Linux container (GNU coreutils 9.11 from source). The
lone DIFF (`du -sb`) was root-caused and **fixed** (see below).

**More bugs the complete matrix caught (the head missed them) — all FIXED:**
the same lowercase-errno bug in **`touch`, `chmod`, `link`, `unlink`** (each had
its own non-capitalizing `reason()` helper, like `ln`) — delegated to
`tool.SysErr`. So the systemic error-string fix now spans cat/ls/ln/touch/chmod/
link/unlink/realpath/diff; the rest were already correct.

**`du -sb` — a real PORTABLE bug, now FIXED (not darwin-specific).** Initial guess
was a darwin/APFS `stat` quirk; the Linux container **disproved** that — it
reproduced identically (GNU dir contribution 0, bashy = the dir's `st_size`) on
Linux too. Root cause: GNU `du --apparent-size` does **not** count a directory's
own `st_size` (proven decisively: GNU `stat` reports the dir as 64B but GNU `du -b`
reports 0 for it), while bashy added it (`du.go:112`). Fix: in `-b` mode a
directory contributes 0 to its *own* apparent size (block mode still counts its
disk blocks). Re-verified 100% on darwin + Linux; `du` unit tests + full coreutils
`go test` green. **NOT a container artifact** — the container only confirmed the
native-darwin finding.

**Remaining / documented findings (not fixed):**
- `uname -a`: bashy omits the `-p` (processor) field GNU prints on darwin — a
  real minor gap (excluded from the committed matrix because its output embeds
  machine identity).
- `hostname`: not shipped by GNU coreutils by default — a reference gap, not a
  bashy bug (excluded).

**Regression gates after the additional fixes:** coreutils `go test`
**0 failures**; the change stays coreutils-only (`sh` untouched) so bash-5.3
compat (86/86) and POSIX (0 bashy-specific failures) are unaffected.

## Next
- `grep -o` (+ `-A/-B/-C` context flags — also found missing) are real coverage gaps worth filling.
- Re-run the complete matrix in the **Linux** bench container to confirm `du -sb` is darwin-only.
- Grow per-command flag depth + a seeded differential fuzzer (GNU as oracle).
