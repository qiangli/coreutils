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
	"hash/fnv"
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

func run(rc *tool.RunContext, args []string) (exitCode int) {
	fs := tool.NewFlags(cmd.Name)
	countStr := fs.StringP("head-count", "n", "", "output at most COUNT lines")
	echo := fs.BoolP("echo", "e", false, "treat each ARG as an input line")
	inputRange := fs.StringP("input-range", "i", "", "treat each number LO through HI as an input line")
	output := fs.StringP("output", "o", "", "write result to FILE instead of standard output")
	repeat := fs.BoolP("repeat", "r", false, "output lines can be repeated")
	randomSource := fs.String("random-source", "", "get random bytes from FILE")
	randomSeed := fs.String("random-seed", "", "use seed to initialize random generator")
	zeroTerminated := fs.BoolP("zero-terminated", "z", false, "line delimiter is NUL, not newline")

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(randomExhaustedError); ok {
				fmt.Fprintf(rc.Err, "shuf: random source exhausted: %v\n", re.err)
				exitCode = 1
			} else {
				panic(r)
			}
		}
	}()

	var seed uint64
	if fs.Changed("random-seed") {
		val, err := strconv.ParseUint(*randomSeed, 10, 64)
		if err == nil {
			seed = val
		} else {
			h := fnv.New64a()
			h.Write([]byte(*randomSeed))
			seed = h.Sum64()
		}
	}

	var rng *rand.Rand
	if fs.Changed("random-source") {
		f, err := os.Open(rc.Path(*randomSource))
		if err != nil {
			fmt.Fprintf(rc.Err, "shuf: %s: %v\n", *randomSource, pathErr(err))
			return 1
		}
		defer f.Close()
		rng = rand.New(&readerSource{r: f})
	} else if fs.Changed("random-seed") {
		rng = rand.New(rand.NewPCG(seed, 0))
	} else {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}

	count := -1 // unlimited
	if fs.Changed("head-count") {
		v, err := strconv.ParseUint(*countStr, 10, 64)
		if err != nil {
			fmt.Fprintf(rc.Err, "shuf: invalid line count: '%s'\n", *countStr)
			return 1
		}
		if v > math.MaxInt {
			v = math.MaxInt
		}
		count = int(v)
	}
	rangeMode := fs.Changed("input-range")
	if *echo && rangeMode {
		return tool.UsageError(rc, cmd, "cannot combine -e and -i options")
	}

	var outWriter io.Writer = rc.Out
	if fs.Changed("output") {
		f, err := os.OpenFile(rc.Path(*output), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			fmt.Fprintf(rc.Err, "shuf: %s: %v\n", *output, pathErr(err))
			return 1
		}
		defer f.Close()
		outWriter = f
	}
	out := bufio.NewWriter(outWriter)
	defer out.Flush()

	lineTerm := "\n"
	if *zeroTerminated {
		lineTerm = "\x00"
	}

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
		n := hi - lo + 1
		fullU64 := lo == 0 && hi == math.MaxUint64
		k := n
		if fullU64 {
			k = math.MaxUint64
		}
		if count >= 0 && uint64(count) < k {
			k = uint64(count)
		}
		if !*repeat && k > maxSampleElems(k == n || fullU64) {
			fmt.Fprintln(rc.Err, "shuf: memory exhausted")
			return 1
		}
		if *repeat {
			if n > 0 {
				for i := 0; count < 0 || i < count; i++ {
					var val uint64
					if fullU64 {
						val = rng.Uint64()
					} else {
						val = lo + rng.Uint64N(n)
					}
					fmt.Fprintf(out, "%d%s", val, lineTerm)
				}
			}
		} else {
			if fullU64 {
				for _, v := range sampleFullU64(rng, k) {
					fmt.Fprintf(out, "%d%s", v, lineTerm)
				}
			} else {
				for _, v := range sampleRange(rng, n, k) {
					fmt.Fprintf(out, "%d%s", lo+v, lineTerm)
				}
			}
		}
	case *echo:
		lines := make([][]byte, len(operands))
		for i, a := range operands {
			lines[i] = []byte(a)
		}
		emitShuffled(out, rng, lines, count, *repeat, lineTerm)
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
		sep := []byte{'\n'}
		if *zeroTerminated {
			sep = []byte{'\x00'}
		}
		if len(data) > 0 {
			data = bytes.TrimSuffix(data, sep)
			lines = bytes.Split(data, sep)
		}
		emitShuffled(out, rng, lines, count, *repeat, lineTerm)
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "shuf: write error: %v\n", err)
		return 1
	}
	return 0
}

type readerSource struct {
	r io.Reader
}

type randomExhaustedError struct {
	err error
}

func (s *readerSource) Uint64() uint64 {
	var b [8]byte
	_, err := io.ReadFull(s.r, b[:])
	if err != nil {
		panic(randomExhaustedError{err: err})
	}
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
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
func emitShuffled(out *bufio.Writer, rng *rand.Rand, lines [][]byte, count int, repeat bool, lineTerm string) {
	n := len(lines)
	if n == 0 {
		return
	}
	if repeat {
		for i := 0; count < 0 || i < count; i++ {
			idx := rng.IntN(n)
			out.Write(lines[idx])
			out.WriteString(lineTerm)
		}
		return
	}
	k := n
	if count >= 0 && count < k {
		k = count
	}
	for i := 0; i < k; i++ {
		j := i + rng.IntN(n-i)
		lines[i], lines[j] = lines[j], lines[i]
	}
	for _, l := range lines[:k] {
		out.Write(l)
		out.WriteString(lineTerm)
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
func sampleRange(rng *rand.Rand, n, k uint64) []uint64 {
	if k == n {
		res := make([]uint64, n)
		for i := range res {
			res[i] = uint64(i)
		}
		rng.Shuffle(len(res), func(a, b int) { res[a], res[b] = res[b], res[a] })
		return res
	}
	chosen := make(map[uint64]struct{}, k)
	res := make([]uint64, 0, k)
	for j := n - k; j < n; j++ {
		t := rng.Uint64N(j + 1)
		if _, dup := chosen[t]; dup {
			t = j
		}
		chosen[t] = struct{}{}
		res = append(res, t)
	}
	rng.Shuffle(len(res), func(a, b int) { res[a], res[b] = res[b], res[a] })
	return res
}

// sampleFullU64 picks k distinct values uniformly from the full uint64
// range (whose span does not fit in uint64 arithmetic). k is small
// here — the memory guard runs first — so rejection sampling is cheap.
func sampleFullU64(rng *rand.Rand, k uint64) []uint64 {
	chosen := make(map[uint64]struct{}, k)
	res := make([]uint64, 0, k)
	for uint64(len(res)) < k {
		v := rng.Uint64()
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
