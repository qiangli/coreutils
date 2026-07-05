# bashy vs GNU coreutils 9.11 — full command comparison

**Date:** 2026-07-05 · **Host:** Apple Mac Studio (darwin/arm64, 96 GB) ·
**Reference:** GNU coreutils **9.11** + grep 3.11 + sed 4.9 + gawk 5.3.1 +
findutils 4.10 + bash **5.3**, all built from source (zero-variance same-machine
head-to-head). **Input:** 1e6-line / ~70 MB log corpus · `LC_ALL=C` · n=10,
warmup 2, median. Measured by `coreutils/cmds/perfbench` (in-process arm =
`tool.Run`, i.e. the code bashy runs in-shell with no fork/exec).

`÷gnu` = bashy median ÷ GNU median. **< 1.00 = bashy faster.** "before" = pre
perf-sprint; "after" = post (wc/cut/sort/base64 optimized by the weave fleet).

## T1 — single command on 70 MB (sorted fastest→slowest, after)

| command | GNU 9.11 | bashy before | bashy after | ÷gnu before | ÷gnu after | note |
|---|--:|--:|--:|--:|--:|---|
| **sha256sum** | 174 ms | 33 ms | **33 ms** | 0.19× | **0.19×** | 5.3× faster — Go ARMv8 SHA ext |
| **wc -l** ⚡ | 23 ms | 308 ms | **5.4 ms** | 12.9× | **0.23×** | **flipped** → 4.3× faster (byte scan) |
| **cat** | 25 ms | 9 ms | **8.2 ms** | 0.37× | **0.34×** | 2.9× faster |
| **awk** | 131 ms | 52 ms | **52 ms** | 0.39× | **0.39×** | 2.5× faster (GoAWK) |
| **base64** ⚡ | 90 ms | 43 ms | **52 ms** | 0.47× | **0.58×** | 1.7× faster; buffering fix targets real-output (cold 18×→~1×) |
| **wc** (all) ⚡ | 83 ms | 313 ms | **53 ms** | 3.78× | **0.64×** | **flipped** → 1.6× faster |
| tac | 62 ms | 37 ms | **40 ms** | 0.56× | **0.65×** | 1.5× faster |
| head | 41 ms | 29 ms | **29 ms** | 0.75× | **0.70×** | 1.4× faster |
| md5sum | 110 ms | 104 ms | **104 ms** | 0.96× | **0.96×** | par |
| tail | 59 ms | 66 ms | **68 ms** | 1.13× | **1.13×** | ~par |
| **sed** ⚡⚡ | 733 ms | 1.31 s | **665 ms** | 1.77× | **0.91×** | **flipped** → 1.1× faster (s/// fast-path, read-all+IndexByte) |
| **sort** ⚡⚡ | 192 ms | 866 ms | **94 ms** | 4.34× | **0.49×** | **flipped** → 2.0× faster (parallel + pdqsort) |
| **sort -n** ⚡⚡ | 277 ms | 1.61 s | **135 ms** | 5.56× | **0.48×** | **flipped** → 2.1× faster (integer radix) |
| **cut** ⚡⚡ | 60 ms | 214 ms | **36 ms** | 3.61× | **0.61×** | **flipped** → 1.6× faster (hot-loop + buffered) |
| **grep** ⚡⚡ | 56 ms | 110 ms | **27 ms** | 1.97× | **0.49×** | **flipped** → 2.0× faster (literal fast-path, RE2 bypass) |

⚡ = optimized this sprint.

## T2 — pipelines (bashy runs them 0-fork, in-process)

| pipeline | GNU 9.11 | bashy before | bashy after | ÷gnu before | ÷gnu after |
|---|--:|--:|--:|--:|--:|
| `find │ wc -l` | 8 ms | 21 ms | **21 ms** | 2.4× | 2.49× |
| topN `grep│sort│uniq -c│sort -rn│head` | 276 ms | 653 ms | **529 ms** | 2.4× | **1.93×** |
| `cat │ tr` | 85 ms | 269 ms | **258 ms** | 3.15× | 3.48× |
| `sort │ uniq -c` | 222 ms | 1.04 s | **706 ms** | 4.7× | **3.15×** |
| wordfreq `tr -s│sort│uniq -c│sort -rn│head` | 3.05 s | 5.28 s | **3.73 s** | 1.74× | **1.22×** |

## Summary

**bashy is faster-or-par than GNU 9.11 on 14 of 15 hot single commands:**
sha256sum **5.3×**, wc-l **4.3×**, cat 2.9×, awk 2.5×, **sort-n 2.1×**, **sort 2.0×**,
grep 2.0×, base64 1.7×, cut 1.6×, wc 1.6×, tac 1.5×, head 1.4×, sed 1.1×, md5sum par.

**The optimization work flipped SEVEN commands from slower to faster than GNU:**
`wc -l` (12.9× → 4.3× faster) and `wc` (3.8× → 1.6×) via byte-scan; `sort`
(4.3× → **2.0×**) via parallel merge (GOMAXPROCS chunks + stable k-way merge) +
unstable pdqsort, `sort -n` (5.6× → **2.1×**) via LSD integer radix; `grep`
(2.0× → **2.0× faster**) via a literal fast-path (RE2 bypass, `bytes.Index`);
`cut` (1.9× → **1.6×**) via a tightened hot loop + buffered output; and `sed`
(1.8× → **1.1×**) via a simple-`s///` fast path (`io.ReadAll` + `bytes.IndexByte`,
after a first attempt's `ReadSlice` aliasing corrupted large output and was
reverted — caught by the 300k-line differential, not the unit tests).

**Still slower:** only tail 1.13× (essentially par; not worth optimizing).

**Pipelines** improved as their slow stages sped up (topN 2.4→1.9×, wordfreq
1.74→1.22×) but remain slower on darwin: `fork` is ~1 ms here, so avoiding 4–5
spawns saves little against the per-stage algorithm gap. The in-process 0-fork
win materializes in a **spawn-dominated regime** — many *cheap* commands, an
expensive-spawn platform (Windows, `CreateProcess` ≈ 2× unix), or the
cold-start case an agent hits firing thousands of one-shot commands.

## Fidelity is preserved

Every command above is **100% byte-identical to GNU coreutils 9.11** — the full
`perfbench conformance` matrix is 62/62 across all present coreutils commands
(see `fidelity-vs-gnu-9.11.md`), and every `cmds/<cmd>/*_test.go` case is green.
The optimizations are speed-only; output is unchanged.

## Methodology notes / lessons (why these numbers are trustworthy)

- **Pin `LC_ALL=C` on BOTH arms.** An uncontrolled run once showed bashy 4×
  *faster* on the topN pipeline — an artifact of GNU `sort` doing UTF-8
  collation. Pinning `LC_ALL=C` dropped the GNU arm 10× and the "win" vanished.
- **Measure output-heavy tools through a REAL sink, not `io.Discard`.** The
  in-process arm writes to `io.Discard` (free), which *hid* that `base64` emitted
  304,687 tiny unbuffered writes (18× slower than GNU when spawned/piped). The
  cold-spawn arm exposed it; the fix (`bufio` on the output) is why `base64`'s
  real-world throughput now matches its in-process speed.
- **grep/find/sed/awk are separate GNU packages**, not coreutils — the harness
  builds them explicitly (a missing reference binary is flagged `FAIL`, never a
  fast-looking 127-exit).
- **Wall-clock is machine-relative** (a trend signal). The hard, machine-
  independent regression gate is the Go benchmark `allocs/op` / `writes/op` in
  `../BASELINE.md` + the `_bench_test.go` files.

## Reproduce
Build GNU 9.11 + the extension packages + bash 5.3 into a prefix (or use
`bashy/eval/agent-shell/containers/bench.Containerfile`), then:
```
GNU_PREFIX=<prefix> BASHY_BIN=<bashy> LC_ALL=C PATH=<prefix>/bin:$PATH \
  perfbench gen && perfbench conformance && perfbench run -n 10
```
