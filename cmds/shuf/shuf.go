// Package shufcmd implements shuf(1) per the GNU coreutils manual:
// write a random permutation of the input lines to standard output.
//
// Randomness comes from math/rand/v2 (auto-seeded): shuf is the
// documented exception to the repo-wide deterministic-output rule —
// random output IS the upstream-documented behavior.
package shufcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "shuf",
	Synopsis: "Write a random permutation of the input lines to standard output.",
	Usage:    "shuf [OPTION]... [FILE]\n   or: shuf -e [OPTION]... [ARG]...\n   or: shuf -i LO-HI [OPTION]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	countStr := fs.StringP("head-count", "n", "", "output at most COUNT lines")
	echo := fs.BoolP("echo", "e", false, "treat each ARG as an input line")
	inputRange := fs.StringP("input-range", "i", "", "treat each number LO through HI as an input line")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	count := -1 // unlimited
	if fs.Changed("head-count") {
		v, err := strconv.ParseUint(*countStr, 10, 64)
		if err != nil {
			fmt.Fprintf(rc.Err, "shuf: invalid line count: '%s'\n", *countStr)
			return 1
		}
		// A count beyond int range means "everything available" — the
		// per-mode k = min(count, n) logic (and the range-mode memory
		// guard) does the rest, as in GNU.
		if v > math.MaxInt {
			v = math.MaxInt
		}
		count = int(v)
	}
	rangeMode := fs.Changed("input-range")
	if *echo && rangeMode {
		return tool.UsageError(rc, cmd, "cannot combine -e and -i options")
	}

	out := bufio.NewWriter(rc.Out)
	switch {
	case rangeMode:
		if len(operands) > 0 {
			return tool.UsageError(rc, cmd, "extra operand '%s'", operands[0])
		}
		lo, hi, ok := parseRange(*inputRange)
		if !ok {
			fmt.Fprintf(rc.Err, "shuf: invalid input range: '%s'\n", *inputRange)
			return 1
		}
		n := hi - lo + 1 // 0 when LO == HI+1 (an allowed empty range)
		// n also wraps to 0 for the full uint64 range 0-MaxUint64,
		// which GNU (unlike uutils) rejects but we must not crash on.
		fullU64 := lo == 0 && hi == math.MaxUint64
		k := n
		if fullU64 {
			k = math.MaxUint64
		}
		if count >= 0 && uint64(count) < k {
			k = uint64(count)
		}
		// GNU reports "memory exhausted" when the sample can't be
		// materialized. Guard BEFORE allocating: an absurd range must
		// degrade into that clean error, not swap the host to death
		// (uutils had the same bug — their #12500).
		if k > maxSampleElems(k == n || fullU64) {
			fmt.Fprintln(rc.Err, "shuf: memory exhausted")
			return 1
		}
		if fullU64 {
			for _, v := range sampleFullU64(k) {
				fmt.Fprintf(out, "%d\n", v)
			}
		} else {
			for _, v := range sampleRange(n, k) {
				fmt.Fprintf(out, "%d\n", lo+v)
			}
		}
	case *echo:
		lines := make([][]byte, len(operands))
		for i, a := range operands {
			lines[i] = []byte(a)
		}
		emitShuffled(out, lines, count)
	default:
		if len(operands) > 1 {
			return tool.UsageError(rc, cmd, "extra operand '%s'", operands[1])
		}
		var r io.Reader = rc.In
		if len(operands) == 1 && operands[0] != "-" {
			f, err := os.Open(rc.Path(operands[0]))
			if err != nil {
				fmt.Fprintf(rc.Err, "shuf: %s: %v\n", operands[0], pathErr(err))
				return 1
			}
			defer f.Close()
			r = f
		}
		data, err := io.ReadAll(r)
		if err != nil {
			fmt.Fprintf(rc.Err, "shuf: read error: %v\n", err)
			return 1
		}
		var lines [][]byte
		if len(data) > 0 {
			data = bytes.TrimSuffix(data, []byte{'\n'})
			lines = bytes.Split(data, []byte{'\n'})
		}
		emitShuffled(out, lines, count)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "shuf: write error: %v\n", err)
		return 1
	}
	return 0
}

// parseRange parses the -i LO-HI argument: unsigned decimal endpoints,
// HI >= LO-1 (GNU allows the empty range LO == HI+1).
func parseRange(s string) (lo, hi uint64, ok bool) {
	dash := strings.IndexByte(s, '-')
	if dash < 0 {
		return 0, 0, false
	}
	var err error
	lo, err = strconv.ParseUint(s[:dash], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	hi, err = strconv.ParseUint(s[dash+1:], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if lo > hi && lo != hi+1 {
		return 0, 0, false
	}
	return lo, hi, true
}

// emitShuffled writes a uniformly random subset of size min(count, len)
// of lines, in random order, via partial Fisher-Yates. count < 0 means
// all lines.
func emitShuffled(out *bufio.Writer, lines [][]byte, count int) {
	n := len(lines)
	k := n
	if count >= 0 && count < k {
		k = count
	}
	for i := 0; i < k; i++ {
		j := i + rand.IntN(n-i)
		lines[i], lines[j] = lines[j], lines[i]
	}
	for _, l := range lines[:k] {
		out.Write(l)
		out.WriteByte('\n')
	}
}

// maxSampleElems bounds how many elements a range sample may
// materialize before shuf reports GNU's "memory exhausted" instead of
// attempting an allocation that could swap the host to death. A full
// permutation costs 8 bytes/elem (plain slice); Floyd sampling adds a
// dedup map (~50 bytes/elem), so its bound is lower.
func maxSampleElems(fullPermutation bool) uint64 {
	if fullPermutation {
		return 1 << 30 // 8 GiB permutation slice
	}
	return 1 << 27 // ~7 GiB Floyd map + slice
}

// sampleRange picks k distinct offsets from [0, n) without
// materializing the range, then shuffles them so the output order is
// uniform too. k == n uses a plain Fisher-Yates permutation (no map);
// k < n uses Floyd's algorithm. Keeps `shuf -i 1-1000000000 -n 5`
// cheap.
func sampleRange(n, k uint64) []uint64 {
	if k == n {
		res := make([]uint64, n)
		for i := range res {
			res[i] = uint64(i)
		}
		rand.Shuffle(len(res), func(a, b int) { res[a], res[b] = res[b], res[a] })
		return res
	}
	chosen := make(map[uint64]struct{}, k)
	res := make([]uint64, 0, k)
	for j := n - k; j < n; j++ {
		t := rand.Uint64N(j + 1)
		if _, dup := chosen[t]; dup {
			t = j
		}
		chosen[t] = struct{}{}
		res = append(res, t)
	}
	rand.Shuffle(len(res), func(a, b int) { res[a], res[b] = res[b], res[a] })
	return res
}

// sampleFullU64 picks k distinct values uniformly from the full uint64
// range (whose span does not fit in uint64 arithmetic). k is small
// here — the memory guard runs first — so rejection sampling is cheap.
func sampleFullU64(k uint64) []uint64 {
	chosen := make(map[uint64]struct{}, k)
	res := make([]uint64, 0, k)
	for uint64(len(res)) < k {
		v := rand.Uint64()
		if _, dup := chosen[v]; dup {
			continue
		}
		chosen[v] = struct{}{}
		res = append(res, v)
	}
	return res
}

// pathErr unwraps *fs.PathError so diagnostics read "shuf: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}
