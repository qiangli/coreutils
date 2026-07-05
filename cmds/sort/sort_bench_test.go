package sortcmd

import (
	"bytes"
	"context"
	"io"
	"math/rand"
	"strconv"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// benchData is ~4 MB of shuffled lines (deterministic seed) so the sort does
// real work.
func benchData() []byte {
	r := rand.New(rand.NewSource(1))
	var b bytes.Buffer
	for b.Len() < 4<<20 {
		b.WriteString(strconv.Itoa(r.Intn(1 << 30)))
		b.WriteByte(' ')
		b.WriteString("tok")
		b.WriteString(strconv.Itoa(r.Intn(10000)))
		b.WriteByte('\n')
	}
	return b.Bytes()
}

func runBench(b *testing.B, args ...string) {
	data := benchData()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			FS:    tool.NewLocalFS(),
			Stdio: tool.Stdio{In: bytes.NewReader(data), Out: io.Discard, Err: io.Discard},
		}
		if code := cmd.Run(rc, args); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}

// BenchmarkSort / BenchmarkSortN are the targets: ~4–6× slower than GNU due to
// io.ReadAll of the whole input + a single sort.SliceStable with per-comparison
// interface overhead + []string allocation. Optimize while keeping every
// sort_test.go case green (byte-identical, stable ordering preserved).
func BenchmarkSort(b *testing.B)  { runBench(b) }
func BenchmarkSortN(b *testing.B) { runBench(b, "-n") }
