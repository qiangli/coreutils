#!/usr/bin/env bash
# lean-bench.sh — deterministic cold/warm-process startup benchmark comparing
# the full multicall binary (cmd/coreutils) against the lean one
# (cmd/coreutils-lean) for the cheap commands a shell-helper path invokes most.
#
# This is the evidence harness for the lean build: the per-invocation cost of a
# cheap command is dominated by mmap'ing and demand-paging the multicall binary,
# not by multicall dispatch (see BenchmarkLeanDispatch* for the sub-microsecond
# in-process path). So the relevant measurement is process startup against a
# cold or warm page cache, which this script produces deterministically.
#
# Usage: scripts/lean-bench.sh [reps]
#   reps   per-command repetitions for the warm median (default 30)
#
# Output: binary sizes, applet counts, and a per-command cold/warm table with the
# full→lean speedup. Exits 0 on success, non-zero if either binary fails to
# build or python3 is unavailable.
#
# Cache clearing: on Linux it tries drop_caches (needs root; warm-only if not
# permitted). On macOS it uses `purge`. When neither is available the cold
# column falls back to a warm single shot.

set -euo pipefail

REPS="${1:-30}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

FULL="$WORK/coreutils-full"
LEAN="$WORK/coreutils-lean"

echo "Building binaries..."
( cd "$ROOT" && go build -o "$FULL" ./cmd/coreutils )
( cd "$ROOT" && go build -o "$LEAN" ./cmd/coreutils-lean )

# Two equal regular files for cmp (it rejects non-regular /dev/null).
printf 'same\n' > "$WORK/cmp-a"; printf 'same\n' > "$WORK/cmp-b"

# Wrap each binary behind a `coreutils` symlink so argv[0] dispatch resolves the
# selfName the multicall front-end expects.
mkdir -p "$WORK/run-full" "$WORK/run-lean"
ln -sf "$FULL" "$WORK/run-full/coreutils"
ln -sf "$LEAN" "$WORK/run-lean/coreutils"

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 1; }

python3 - "$WORK/run-full/coreutils" "$WORK/run-lean/coreutils" "$WORK/cmp-a" "$WORK/cmp-b" "$REPS" <<'PY'
import os, statistics, subprocess, sys, time

full, lean, cmp_a, cmp_b, reps = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], int(sys.argv[5])

def mb(path):
    return os.path.getsize(path) / 1048576.0

def applets(path):
    return subprocess.run([path, "--list"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                          text=True).stdout.count("\n")

print()
print(f"{'binary':<10} {'size':>8} {'applets':>8}")
print(f"{'full':<10} {mb(full):>6.1f}MB {applets(full):>8}")
print(f"{'lean':<10} {mb(lean):>6.1f}MB {applets(lean):>8}")
print(f"size reduction: {mb(full)/mb(lean):.1f}x smaller")

def drop_cache():
    # macOS
    subprocess.run(["purge"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    # Linux (best-effort; needs root)
    try:
        with open("/proc/sys/vm/drop_caches", "w") as f:
            os.sync(); f.write("3\n")
    except OSError:
        pass

def warm(path, args, n):
    for _ in range(5):
        subprocess.run([path]+args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    t = []
    for _ in range(n):
        t0 = time.perf_counter()
        subprocess.run([path]+args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        t.append((time.perf_counter()-t0)*1000)
    return statistics.median(t)

def cold(path, args):
    drop_cache()
    t0 = time.perf_counter()
    subprocess.run([path]+args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return (time.perf_counter()-t0)*1000

tests = [("true", ["true"]), ("expr", ["expr","1","+","2"]),
         ("test", ["test","1","-eq","1"]), ("echo", ["echo","x"]),
         ("cmp", ["cmp", cmp_a, cmp_b])]

print()
print(f"{'cmd':<6} {'mode':<5} {'full':>9} {'lean':>9} {'speedup':>8}")
for name, args in tests:
    fw, lw = warm(full, args, reps), warm(lean, args, reps)
    print(f"{name:<6} {'warm':<5} {fw:>7.2f}ms {lw:>7.2f}ms {fw/lw:>7.2f}x")
    fc, lc = cold(full, args), cold(lean, args)
    print(f"{name:<6} {'cold':<5} {fc:>7.2f}ms {lc:>7.2f}ms {fc/lc:>7.2f}x")
    print()
print(f"Done. (warm = {reps}-rep median; cold = single shot after cache drop)")
PY
