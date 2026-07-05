package sortcmd

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// runSort runs the full command over input and returns stdout bytes.
func runSort(t *testing.T, input []byte, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		FS:    tool.NewLocalFS(),
		Stdio: tool.Stdio{In: bytes.NewReader(input), Out: &out, Err: &out},
	}
	if code := cmd.Run(rc, args); code != 0 {
		t.Fatalf("sort %v: exit %d\n%s", args, code, out.Bytes())
	}
	return out.Bytes()
}

// intLines: integer keys only (hits the radix path), with duplicates,
// negatives, leading zeros, "-0", "0.000" (integer after zero-strip),
// non-numeric and empty lines, and duplicate whole lines.
func intLines(r *rand.Rand, n int) []byte {
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		switch r.Intn(12) {
		case 0:
			b.WriteString("-")
			b.WriteString(strconv.Itoa(r.Intn(1000)))
		case 1:
			fmt.Fprintf(&b, "%05d trailing", r.Intn(1000))
		case 2:
			b.WriteString("-0")
		case 3:
			b.WriteString("0.000")
		case 4:
			b.WriteString("junk line ")
			b.WriteString(strconv.Itoa(r.Intn(5)))
		case 5:
			// nothing: empty line
		case 6:
			b.WriteString("  \t ")
			b.WriteString(strconv.Itoa(r.Intn(1000)))
		default:
			b.WriteString(strconv.Itoa(r.Intn(1000) - 500))
			b.WriteString(" tok")
			b.WriteString(strconv.Itoa(r.Intn(50)))
		}
		b.WriteByte('\n')
	}
	return b.Bytes()
}

// mixedLines adds fractional values so the radix path must decline and
// the parallel comparison sort runs instead.
func mixedLines(r *rand.Rand, n int) []byte {
	b := intLines(r, n-n/4)
	var t bytes.Buffer
	for i := 0; i < n/4; i++ {
		fmt.Fprintf(&t, "%d.%03d frac\n", r.Intn(200)-100, r.Intn(1000))
	}
	return append(b, t.Bytes()...)
}

// textLines: word lines with heavy field-2 duplication so -k keys tie
// often (stresses stability and the last-resort tiebreak).
func textLines(r *rand.Rand, n int) []byte {
	words := []string{"alpha", "Beta", "gamma", "DELTA", "epsilon", "zeta", "", "Alpha"}
	var b bytes.Buffer
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%s %s %d\n", words[r.Intn(len(words))], words[r.Intn(len(words))], r.Intn(30))
	}
	return b.Bytes()
}

// TestFastSortByteIdentical pins every fast path (unstable pdqsort,
// parallel chunk sort + merge, integer radix) byte-for-byte against
// the pre-optimization algorithm (serial SortStableFunc with the same
// comparators), on inputs both below and above parallelMinLines.
func TestFastSortByteIdentical(t *testing.T) {
	if testing.Short() {
		t.Skip("large differential inputs")
	}
	combos := [][]string{
		{},
		{"-r"},
		{"-u"},
		{"-s"},
		{"-n"},
		{"-n", "-r"},
		{"-n", "-u"},
		{"-n", "-s"},
		{"-n", "-r", "-u"},
		{"-n", "-r", "-s"},
		{"-k1n"},       // prepared-numeric via explicit key
		{"-k1n", "-r"}, // keyRev != global reverse: radix must decline
		{"-k2"},
		{"-k2", "-s"},
		{"-k2", "-u"},
		{"-k2", "-r"},
		{"-k2,2", "-k1,1r"},
		{"-k3n", "-k1,1"},
		{"-t", " ", "-k2"},
		{"-f"},
		{"-b", "-k2"},
	}
	sizes := []int{500, 60_000} // below and above parallelMinLines
	gens := []struct {
		name string
		gen  func(*rand.Rand, int) []byte
	}{
		{"int", intLines},
		{"mixed", mixedLines},
		{"text", textLines},
	}
	for _, g := range gens {
		for _, n := range sizes {
			input := g.gen(rand.New(rand.NewSource(int64(n))), n)
			for _, args := range combos {
				name := fmt.Sprintf("%s/%d/%s", g.name, n, strings.Join(args, "_"))
				t.Run(name, func(t *testing.T) {
					serialStableForTest = true
					want := runSort(t, input, args...)
					serialStableForTest = false
					got := runSort(t, input, args...)
					if !bytes.Equal(got, want) {
						t.Fatalf("output differs from serial stable sort (len got %d, want %d)", len(got), len(want))
					}
				})
			}
		}
	}
}
