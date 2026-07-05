# perfbench baseline — regression reference

Post-optimization performance baseline for the hot coreutils commands.
Committed alongside the harness so it travels with the code and gates regressions.

**Detailed side-by-side results:** `results/bashy-vs-gnu-9.11.md` (full per-command
timing, all 15 T1 + 5 T2 workloads, before/after/vs-GNU) · `results/fidelity-vs-gnu-9.11.md`
(62/62 byte-identical conformance matrix). This file is the terse regression gate.

**Established:** 2026-07-05 (perf-sprint: wc/sort/cut/base64 optimized by the
weave fleet). **Method:** `go test -bench=. -benchmem ./cmds/<cmd>/` — the
`_bench_test.go` files are the executable guard. Machine: Apple M-series
(darwin/arm64), go 1.26.

## Why these numbers gate regressions

- **`allocs/op` and `writes/op` are machine-INDEPENDENT** — they must not
  increase. These are the primary regression signals.
- **`ns/op` / `MB/s` are machine-relative** — compare ratios, not absolutes;
  regress only on a large (>1.5×) slowdown on the same machine.

## Baseline (optimized) — the numbers to hold

| benchmark | ns/op | throughput | B/op | allocs/op | gate (must not exceed) |
|---|--:|--:|--:|--:|---|
| `BenchmarkWCLines` (wc -l, 10 MB) | 374 K | 28 GB/s | 72,786 | **35** | allocs ≤ 40 · ns < 2 M |
| `BenchmarkWCAll` (wc, 10 MB) | 4.1 M | 2.55 GB/s | 72,528 | **34** | allocs ≤ 40 |
| `BenchmarkSort` (4 MB) | 11.9 M | 353 MB/s | 17.5 M | **129** | allocs ≤ 160 · ns < 20 M |
| `BenchmarkSortN` (4 MB) | 11.5 M | 365 MB/s | 36.3 M | **90** | allocs ≤ 110 · ns < 20 M |
| `BenchmarkCutFields` (10 MB) | 8.4 M | 1.25 GB/s | ~11 K | **43** | allocs ≤ 60 · ns < 12 M |
| `BenchmarkGrepLiteral` (10 MB) | 1.5 M | 7 GB/s | 574 K | **140** | allocs ≤ 300 · ns < 3 M |
| `BenchmarkSedSubst` (10 MB) | 41.3 M | 254 MB/s | ~ | **551,996** | allocs ≤ 700 K · ns < 55 M |
| `BenchmarkCutChars` (10 MB) | 14.5 M | 723 MB/s | 10,853 | **35** | allocs ≤ 60 |
| `BenchmarkBase64Writes` (8 MB) | 4.2 M | 2.0 GB/s | 68,616 | 27 · **173 writes** | writes ≤ 1000 |

## What changed (before → after, same harness)

| command | before | after | win | root fix |
|---|--:|--:|--:|---|
| **wc -l** | 35.7 M ns, 293 MB/s | **374 K ns, 28 GB/s** | **95×** | byte scan (`bytes.Count`) replaces rune-by-rune `ReadRune`; rune path kept for `-m`/`-L` |
| **wc (all)** | 35.6 M ns | **4.1 M ns** | **8.7×** | byte-scan word/line/byte fast path |
| **cut** | 21 M ns, **374,530 allocs** | **10.7 M ns, 38 allocs** | 2×, **9856× fewer allocs** | `bytes.IndexByte` field scan + buffered output, no `[]string` per line |
| **sort** | 82 M ns | **11.9 M ns** | **6.9×** | parallel merge (GOMAXPROCS chunks + stable k-way merge) + unstable pdqsort (default) + `slices.SortStableFunc` keys |
| **sort -n** | 174 M ns | **11.5 M ns** | **15×** | LSD radix sort on int64 keys (bit-flipped for descending/negatives) + parallel |
| **base64** | **304,687 writes** | **173 writes** | **1761×** | `bufio.NewWriterSize(64K)` on encode+decode (was unbuffered — 18× slower on a real pipe/file, invisible to `io.Discard`) |
| **grep** (literal) | 9.1 M ns, **254,988 allocs** | **1.5 M ns, 140 allocs** | **6.1×**, 1821× fewer allocs | literal fast-path (`bytes.Index`, RE2 bypass) for safe modes |
| **cut** (round 2) | 10.7 M ns | **8.4 M ns** | 1.28× | tightened field scan + buffered output |
| **sed** | 66.7 M ns, **2,024,955 allocs** | **41.3 M ns, 551,996 allocs** | **1.6×**, 3.7× fewer allocs | simple-`s///` fast path (`io.ReadAll` + `bytes.IndexByte`, buffered) |

**Fidelity preserved:** every command still 100% byte-identical to GNU coreutils
9.11 (perfbench conformance matrix, 62/62) and every `cmds/<cmd>/*_test.go` case
green. Optimizations are speed-only, output unchanged.

## vs GNU coreutils 9.11 (wall-clock, in-process, darwin/arm64, 70 MB, `LC_ALL=C`, n=10)

Measured `perfbench run` bashy-in-process vs GNU coreutils 9.11 (=1.00). Ratio
<1.00 = bashy faster. The optimization rounds **flipped seven commands from
slower to faster than GNU**:

| command | pre-opt ÷gnu | **post-opt ÷gnu** | verdict |
|---|--:|--:|---|
| **wc -l** | 12.9× slower | **0.23× — 4.3× FASTER** | flipped |
| **wc** (all) | 3.8× slower | **0.64× — 1.6× FASTER** | flipped |
| **sort** | 4.3× slower | **0.49× — 2.0× FASTER** | flipped (parallel + pdqsort) |
| **sort -n** | 5.6× slower | **0.48× — 2.1× FASTER** | flipped (integer radix) |
| **base64** (in-proc) | 0.47× faster | **0.58× faster** | already fast; buffering fix targets real-output/cold (18×→~1×) |
| **grep** | 2.0× slower | **0.49× — 2.0× FASTER** | flipped (literal fast-path) |
| **cut** | 3.6× slower | **0.61× — 1.6× FASTER** | flipped (round 2: hot-loop + buffered) |
| **sed** | 1.8× slower | **0.91× — 1.1× FASTER** | flipped (s/// fast-path) |

**bashy faster-or-par than GNU on 14 of 15 hot commands** (post all rounds):
sha256sum **0.19×**, wc-l **0.23×**, cat 0.34×, awk 0.39×, **sort-n 0.48×**, **sort 0.49×**,
**grep 0.49×**, base64 0.58×, **cut 0.61×**, wc **0.64×**, tac 0.65×, head 0.70×,
**sed 0.91×**, md5sum 0.96×. Only tail (1.13×) remains nominally slower (~par).

Pipelines (T2, 0-fork): topN 2.4×→**1.93×**, wordfreq 1.74×→**1.22×** — improved as
the slow stages sped up, still slower on darwin (cheap fork ⇒ 0-fork win is small;
the win is a spawn-dominated / Windows regime — see the strategy doc §2).

*Wall-clock is machine-relative (trend signal); the Go benchmarks above are the
hard, machine-independent gate. Re-measure with the bench container or a native
GNU-9.11 host — `docs/coreutils-fidelity-perf-harness-spec.md` §6.*

## Re-measure / detect regression
```
go test -run='^$' -bench=. -benchmem ./cmds/wc/ ./cmds/sort/ ./cmds/cut/ ./cmds/base64/ ./cmds/grep/ ./cmds/sed/
```
Compare `allocs/op` / `writes/op` against the gate column above (hard);
`ns/op` against the same-machine baseline (soft, >1.5× = investigate).
`benchstat old.txt new.txt` for statistical comparison in CI.
