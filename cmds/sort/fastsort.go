// Sort-algorithm selection: unstable pdqsort where the comparator is a
// total order, parallel chunk sort + stable k-way merge above a size
// threshold, and an LSD radix sort for the all-integer numeric fast
// path. Every path is byte-identical to a serial stable sort of the
// same comparator:
//
//   - unstable is used only where compare(a,b)==0 implies a and b are
//     byte-identical lines (the whole-line last-resort tiebreak makes
//     the default orders total), so any sorted permutation prints the
//     same bytes;
//   - the merge takes from the earlier run on ties, so stably sorted
//     contiguous chunks merge into exactly the serial stable order.
package sortcmd

import (
	"runtime"
	"slices"
	"strings"
	"sync"
)

// parallelMinLines is the input size below which goroutine + merge
// overhead loses to a plain serial sort.
const parallelMinLines = 50_000

// serialStableForTest forces the pre-optimization algorithm (a serial
// slices.SortStableFunc with the unchanged comparators) so the
// differential test can pin byte-identity of every fast path.
var serialStableForTest = false

// sortLines orders the string path. Without keys the comparator is
// plain whole-line byte order (± global -r) for every flag combination,
// so the specialized slices.Sort runs and -r reverses the result (the
// exact reversal of a total order). With keys, -s/-u drop the
// last-resort tiebreak and distinct lines can compare equal, so those
// stay stable; otherwise the order is total and pdqsort is safe.
func (s *sorter) sortLines(lines []string) {
	if serialStableForTest {
		slices.SortStableFunc(lines, s.compare)
		return
	}
	if len(s.keys) == 0 {
		parallelSort(lines, strings.Compare, slices.Sort)
		if s.reverse {
			slices.Reverse(lines)
		}
		return
	}
	stable := s.stable || s.unique
	parallelSort(lines, s.compare, func(x []string) {
		if stable {
			slices.SortStableFunc(x, s.compare)
		} else {
			slices.SortFunc(x, s.compare)
		}
	})
}

// sortNumericLines orders the prepared-numeric fast path. When every
// line's key is an integer that fits int64, an LSD radix sort on the
// precomputed values replaces the comparison sort; otherwise the same
// parallel machinery as the string path runs with the numeric
// comparator.
func (s *sorter) sortNumericLines(lines []numericLine, allInt bool) {
	if serialStableForTest {
		slices.SortStableFunc(lines, s.compareNumericLines)
		return
	}
	stable := s.stable || s.unique
	keyRev := s.keys[0].opts.reverse
	if allInt {
		if stable {
			// Order is by key only, ties keep input order: exactly a
			// stable radix sort (descending via bit-flipped keys).
			radixSortNumeric(lines, keyRev)
			return
		}
		if keyRev == s.reverse {
			// Total order (key, whole line), both reversed together:
			// sort ascending, then reverse the whole slice for -r.
			radixSortNumeric(lines, false)
			sortIntTieRuns(lines)
			if s.reverse {
				slices.Reverse(lines)
			}
			return
		}
		// keyRev != s.reverse (per-key r vs global -r mismatch): the
		// two directions apply to different comparison levels; keep
		// the comparison sort.
	}
	parallelSort(lines, s.compareNumericLines, func(x []numericLine) {
		if stable {
			slices.SortStableFunc(x, s.compareNumericLines)
		} else {
			slices.SortFunc(x, s.compareNumericLines)
		}
	})
}

// parallelSort sorts x, using chunkSort for the serial pieces and cmp
// for merging. Below the threshold (or on one CPU) it is exactly
// chunkSort(x). Above it, GOMAXPROCS contiguous chunks are chunkSorted
// concurrently and pairwise-merged; mergeRuns prefers the earlier run
// on ties, so the result equals the serial chunkSort order whether
// chunkSort is stable or an unstable sort of a total order.
func parallelSort[T any](x []T, cmp func(a, b T) int, chunkSort func([]T)) {
	n := len(x)
	p := runtime.GOMAXPROCS(0)
	if n < parallelMinLines || p < 2 {
		chunkSort(x)
		return
	}
	bounds := make([]int, p+1)
	for i := range bounds {
		bounds[i] = i * n / p
	}
	var wg sync.WaitGroup
	for i := 0; i < p; i++ {
		lo, hi := bounds[i], bounds[i+1]
		wg.Add(1)
		go func() {
			defer wg.Done()
			chunkSort(x[lo:hi])
		}()
	}
	wg.Wait()

	buf := make([]T, n)
	src, dst := x, buf
	for len(bounds) > 2 {
		next := make([]int, 0, len(bounds)/2+2)
		next = append(next, 0)
		var mg sync.WaitGroup
		i := 0
		for ; i+2 < len(bounds); i += 2 {
			lo, mid, hi := bounds[i], bounds[i+1], bounds[i+2]
			mg.Add(1)
			go func() {
				defer mg.Done()
				mergeRuns(dst[lo:hi], src[lo:mid], src[mid:hi], cmp)
			}()
			next = append(next, hi)
		}
		if i+2 == len(bounds) { // odd run count: carry the last run over
			lo, hi := bounds[i], bounds[i+1]
			copy(dst[lo:hi], src[lo:hi])
			next = append(next, hi)
		}
		mg.Wait()
		bounds = next
		src, dst = dst, src
	}
	if n > 0 && &src[0] != &x[0] {
		copy(x, src)
	}
}

// mergeRuns merges sorted runs a and b into dst (len(dst) ==
// len(a)+len(b)), taking from a on ties: with a preceding b in the
// input, that is the stable merge.
func mergeRuns[T any](dst, a, b []T, cmp func(x, y T) int) {
	i, j, k := 0, 0, 0
	for i < len(a) && j < len(b) {
		if cmp(a[i], b[j]) <= 0 {
			dst[k] = a[i]
			i++
		} else {
			dst[k] = b[j]
			j++
		}
		k++
	}
	copy(dst[k:], a[i:])
	copy(dst[k+len(a)-i:], b[j:])
}

// intKeyVal returns the key's exact int64 value when it is an integer
// (no fractional digits after trailing-zero stripping) of at most 18
// digits, which cannot overflow. Values order identically to
// compareNumericKey over integer keys: sign first (non-numbers and
// zeros are sign 0 → value 0), then magnitude (ipLen + digits with
// leading zeros stripped).
func intKeyVal(s string, k numericKey) (int64, bool) {
	if k.fpLen != 0 || k.ipLen > 18 {
		return 0, false
	}
	var v int64
	for i := k.ipStart; i < k.ipStart+k.ipLen; i++ {
		v = v*10 + int64(s[i]-'0')
	}
	if k.sign < 0 {
		v = -v
	}
	return v, true
}

// radixSortNumeric stably sorts lines by their precomputed int64 value
// with an LSD radix sort (8-bit digits, sign bit biased so unsigned
// byte order matches signed order; desc flips all bits). Byte
// positions where every key agrees are skipped, so small-magnitude
// inputs pay only for the bytes that vary.
func radixSortNumeric(lines []numericLine, desc bool) {
	n := len(lines)
	if n < 2 {
		return
	}
	keys := make([]uint64, n)
	for i := range lines {
		k := uint64(lines[i].val) ^ (1 << 63)
		if desc {
			k = ^k
		}
		keys[i] = k
	}
	var counts [8][256]int
	for _, k := range keys {
		for b := 0; b < 8; b++ {
			counts[b][byte(k>>(b*8))]++
		}
	}
	src, dst := lines, make([]numericLine, n)
	ksrc, kdst := keys, make([]uint64, n)
	var offs [256]int
	for b := 0; b < 8; b++ {
		c := &counts[b]
		shift := uint(b * 8)
		if c[byte(ksrc[0]>>shift)] == n { // every key shares this byte
			continue
		}
		sum := 0
		for v := 0; v < 256; v++ {
			offs[v] = sum
			sum += c[v]
		}
		for i := 0; i < n; i++ {
			v := byte(ksrc[i] >> shift)
			o := offs[v]
			offs[v]++
			kdst[o] = ksrc[i]
			dst[o] = src[i]
		}
		src, dst = dst, src
		ksrc, kdst = kdst, ksrc
	}
	if &src[0] != &lines[0] {
		copy(lines, src)
	}
}

// sortIntTieRuns applies the last-resort whole-line comparison inside
// each run of equal integer values (equal value ⇔ equal numeric key
// for integer keys), completing the (key, whole line) total order
// after a radix sort.
func sortIntTieRuns(lines []numericLine) {
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && lines[j].val == lines[i].val {
			j++
		}
		if j-i > 1 {
			slices.SortFunc(lines[i:j], func(a, b numericLine) int {
				return strings.Compare(a.text, b.text)
			})
		}
		i = j
	}
}
