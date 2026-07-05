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
| `BenchmarkSort` (4 MB) | 57.7 M | 73 MB/s | 13.8 M | **82** | allocs ≤ 90 · B/op ≤ 15 M |
| `BenchmarkSortN` (4 MB) | 111 M | 38 MB/s | 19.4 M | **92** | allocs ≤ 100 |
| `BenchmarkCutFields` (10 MB) | 10.7 M | 982 MB/s | 10,997 | **38** | allocs ≤ 60 |
| `BenchmarkCutChars` (10 MB) | 14.5 M | 723 MB/s | 10,853 | **35** | allocs ≤ 60 |
| `BenchmarkBase64Writes` (8 MB) | 4.2 M | 2.0 GB/s | 68,616 | 27 · **173 writes** | writes ≤ 1000 |

## What changed (before → after, same harness)

| command | before | after | win | root fix |
|---|--:|--:|--:|---|
| **wc -l** | 35.7 M ns, 293 MB/s | **374 K ns, 28 GB/s** | **95×** | byte scan (`bytes.Count`) replaces rune-by-rune `ReadRune`; rune path kept for `-m`/`-L` |
| **wc (all)** | 35.6 M ns | **4.1 M ns** | **8.7×** | byte-scan word/line/byte fast path |
| **cut** | 21 M ns, **374,530 allocs** | **10.7 M ns, 38 allocs** | 2×, **9856× fewer allocs** | `bytes.IndexByte` field scan + buffered output, no `[]string` per line |
| **sort** | 82 M ns, 21.7 MB | **57.7 M ns, 13.8 MB** | 1.4×, −37% mem | reduced per-line sorting overhead + allocation |
| **sort -n** | 174 M ns | **111 M ns** | 1.6× | " |
| **base64** | **304,687 writes** | **173 writes** | **1761×** | `bufio.NewWriterSize(64K)` on encode+decode (was unbuffered — 18× slower on a real pipe/file, invisible to `io.Discard`) |

**Fidelity preserved:** every command still 100% byte-identical to GNU coreutils
9.11 (perfbench conformance matrix, 62/62) and every `cmds/<cmd>/*_test.go` case
green. Optimizations are speed-only, output unchanged.

## vs GNU coreutils 9.11 (wall-clock, in-process, darwin/arm64, 70 MB, `LC_ALL=C`, n=10)

Measured `perfbench run` bashy-in-process vs GNU coreutils 9.11 (=1.00). Ratio
<1.00 = bashy faster. The sprint **flipped the two worst offenders to faster
than GNU** and halved the others' gaps:

| command | pre-opt ÷gnu | **post-opt ÷gnu** | verdict |
|---|--:|--:|---|
| **wc -l** | 12.9× slower | **0.23× — 4.3× FASTER** | flipped |
| **wc** (all) | 3.8× slower | **0.64× — 1.6× FASTER** | flipped |
| **cut** | 3.6× slower | **1.89× slower** | gap ~halved |
| **sort** | 4.3× slower | **2.60× slower** | improved (io.ReadAll ceiling remains) |
| **sort -n** | 5.6× slower | **3.43× slower** | improved |
| **base64** (in-proc) | 0.47× faster | **0.58× faster** | already fast; buffering fix targets real-output/cold (18×→~1×) |

**bashy faster-or-par than GNU on 9 of 15 hot commands** (post-sprint):
sha256sum **0.19×**, cat 0.34×, awk 0.39×, base64 0.58×, wc-l **0.23×**, wc **0.64×**,
tac 0.65×, head 0.70×, md5sum 0.96×. Still slower (untargeted, or algorithmic):
tail 1.13×, sed 1.80×, cut 1.89×, grep 1.99×, sort 2.60×, sort-n 3.43×.

Pipelines (T2, 0-fork): topN 2.4×→**1.93×**, wordfreq 1.74×→**1.22×** — improved as
the slow stages sped up, still slower on darwin (cheap fork ⇒ 0-fork win is small;
the win is a spawn-dominated / Windows regime — see the strategy doc §2).

*Wall-clock is machine-relative (trend signal); the Go benchmarks above are the
hard, machine-independent gate. Re-measure with the bench container or a native
GNU-9.11 host — `docs/coreutils-fidelity-perf-harness-spec.md` §6.*

## Re-measure / detect regression
```
go test -run='^$' -bench=. -benchmem ./cmds/wc/ ./cmds/sort/ ./cmds/cut/ ./cmds/base64/
```
Compare `allocs/op` / `writes/op` against the gate column above (hard);
`ns/op` against the same-machine baseline (soft, >1.5× = investigate).
`benchstat old.txt new.txt` for statistical comparison in CI.
