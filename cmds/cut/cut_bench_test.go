package cutcmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func benchData() []byte {
	line := []byte("field1 field2 field3 field4 field5 field6 field7 field8\n")
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

// BenchmarkCutFields is the target: ~3.6× slower than GNU — per-line field
// splitting/allocation. Optimize while keeping every cut_test.go case green.
func BenchmarkCutFields(b *testing.B) { runBench(b, "-d", " ", "-f", "1,3,5") }
func BenchmarkCutChars(b *testing.B)  { runBench(b, "-c", "1-10") }
