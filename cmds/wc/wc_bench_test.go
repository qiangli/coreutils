package wccmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// benchData is ~10 MB of representative log lines (deterministic).
func benchData() []byte {
	line := []byte("0000000042 INFO tok123 tok456 tok789 alpha beta gamma\n")
	var b bytes.Buffer
	for b.Len() < 10<<20 {
		b.Write(line)
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

// BenchmarkWCLines is the primary target: `wc -l` over ~10 MB — currently
// ~13× slower than GNU due to rune-by-rune ReadRune. Optimize to a byte scan
// while keeping every wc_test.go case green (byte-identical output).
func BenchmarkWCLines(b *testing.B) { runBench(b, "-l") }
func BenchmarkWCAll(b *testing.B)   { runBench(b) }
