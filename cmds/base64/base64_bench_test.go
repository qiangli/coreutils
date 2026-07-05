package base64cmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func benchData() []byte {
	var b bytes.Buffer
	chunk := bytes.Repeat([]byte("abcdefghijklmnop"), 64) // 1 KiB
	for b.Len() < 8<<20 {                                 // ~8 MB
		b.Write(chunk)
	}
	return b.Bytes()
}

// countWriter counts Write() calls — the signal for the output-buffering bug:
// unbuffered base64 emits millions of tiny writes (catastrophic on a real
// pipe/file, though free to io.Discard). Buffering the output collapses this to
// a handful of large writes.
type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n++; return len(p), nil }

// BenchmarkBase64Writes reports writes/op — the metric to DRIVE DOWN. Encode
// speed itself is already faster than GNU; the target is the write count.
// Keep every base64_test.go case green (byte-identical output).
func BenchmarkBase64Writes(b *testing.B) {
	data := benchData()
	var writes int64
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cw := &countWriter{}
		rc := &tool.RunContext{
			Ctx:   context.Background(),
			FS:    tool.NewLocalFS(),
			Stdio: tool.Stdio{In: bytes.NewReader(data), Out: cw, Err: io.Discard},
		}
		if code := cmd.Run(rc, nil); code != 0 {
			b.Fatalf("exit %d", code)
		}
		writes = cw.n
	}
	b.ReportMetric(float64(writes), "writes/op")
}
